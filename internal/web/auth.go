package web

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cg/internal/auth"
	"cg/internal/storage"
)

const sessionCookie = "cg_session"
const sessionLifetime = 24 * time.Hour

type sessionContextKey struct{}

func requestSession(r *http.Request) storage.Session {
	session, _ := r.Context().Value(sessionContextKey{}).(storage.Session)
	return session
}

func (s *Server) readSession(r *http.Request) (storage.Session, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || len(cookie.Value) != 43 {
		return storage.Session{}, sql.ErrNoRows
	}
	return s.store.LoadSession(r.Context(), cookie.Value)
}

func (s *Server) statusPrivate(ctx context.Context) (bool, error) {
	if s.admin == nil {
		return s.cfg.StatusLoginRequired, nil
	}
	cfg, err := s.admin.AdminConfig(ctx)
	return cfg.Settings.StatusLoginRequired, err
}

func (s *Server) streamAuthorized(r *http.Request) bool {
	private, err := s.statusPrivate(r.Context())
	if err != nil {
		return false
	}
	if !private {
		return true
	}
	session, err := s.readSession(r)
	return err == nil && !session.User.MustChangePassword
}

func (s *Server) requireStatusAccess(w http.ResponseWriter, r *http.Request) bool {
	private, err := s.statusPrivate(r.Context())
	if err != nil {
		writeErrorText(w, http.StatusServiceUnavailable, "暂时无法读取访问策略")
		return false
	}
	return !private || s.requireAuth(w, r)
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	return s.authorize(w, r, false, false)
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	return s.authorize(w, r, true, false)
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request, adminOnly, allowPasswordChange bool) bool {
	session, err := s.readSession(r)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			writeErrorText(w, http.StatusServiceUnavailable, "暂时无法验证会话")
			return false
		}
		writeErrorText(w, http.StatusUnauthorized, "请先登录")
		return false
	}
	*r = *r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, session))
	if session.User.MustChangePassword && !allowPasswordChange {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "请先修改初始密码", "code": "password_change_required"})
		return false
	}
	if adminOnly && session.User.Role != "admin" {
		writeErrorText(w, http.StatusForbidden, "仅管理员可执行此操作")
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if !s.sameOrigin(w, r) {
			return false
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CSRFToken)) != 1 {
			writeErrorText(w, http.StatusForbidden, "会话校验失败，请刷新页面后重试")
			return false
		}
	}
	return true
}

func (s *Server) sameOrigin(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		writeErrorText(w, http.StatusForbidden, "不允许跨站请求")
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		scheme := "http"
		if r.TLS != nil || s.cfg.SecureCookies {
			scheme = "https"
		}
		if err != nil || u.Scheme != scheme || !strings.EqualFold(u.Host, r.Host) || u.User != nil || u.Path != "" {
			writeErrorText(w, http.StatusForbidden, "不允许跨站请求")
			return false
		}
	}
	return true
}

func clientAddress(r *http.Request) string {
	address := r.RemoteAddr
	if host, _, err := net.SplitHostPort(address); err == nil {
		address = host
	}
	return address
}

func (s *Server) passwordAttempt(w http.ResponseWriter, r *http.Request) (func(), bool) {
	address := clientAddress(r)
	if delay := s.authFailures.retryAfter(address, time.Now()); delay > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(delay.Seconds())+1))
		writeErrorText(w, http.StatusTooManyRequests, "登录尝试过于频繁，请稍后重试")
		return nil, false
	}
	select {
	case s.passwordSlots <- struct{}{}:
		// Count before running the expensive KDF so parallel attempts are bounded too.
		s.authFailures.fail(address, time.Now())
		return func() { <-s.passwordSlots }, true
	default:
		w.Header().Set("Retry-After", "2")
		writeErrorText(w, http.StatusTooManyRequests, "认证服务繁忙，请稍后重试")
		return nil, false
	}
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	maxAge := int(sessionLifetime.Seconds())
	if token == "" {
		maxAge = -1
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure: s.cfg.SecureCookies || r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: maxAge, Expires: expires})
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, user storage.User) {
	token, err := randomToken(32)
	if err != nil {
		writeErrorText(w, 500, "无法创建会话")
		return
	}
	csrf, err := randomToken(32)
	if err != nil {
		writeErrorText(w, 500, "无法创建会话")
		return
	}
	expires := time.Now().Add(sessionLifetime)
	if err := s.store.CreateSession(r.Context(), user, token, csrf, expires); err != nil {
		writeErrorText(w, http.StatusUnauthorized, "账号已变更，请重新登录")
		return
	}
	if old, err := r.Cookie(sessionCookie); err == nil {
		_ = s.store.DeleteSession(r.Context(), old.Value)
	}
	s.setSessionCookie(w, r, token, expires)
	*r = *r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, storage.Session{User: user}))
	s.writeSession(w, r, storage.Session{User: user, CSRFToken: csrf, ExpiresAt: expires.Unix()})
}

func (s *Server) writeSession(w http.ResponseWriter, r *http.Request, session storage.Session) {
	private, err := s.statusPrivate(r.Context())
	if err != nil {
		writeErrorText(w, http.StatusServiceUnavailable, "暂时无法读取访问策略")
		return
	}
	var user *storage.User
	if session.User.ID != 0 {
		user = &session.User
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "csrf_token": session.CSRFToken, "expires_at": session.ExpiresAt, "status_login_required": private})
}

func (s *Server) authSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	session, err := s.readSession(r)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeErrorText(w, http.StatusServiceUnavailable, "暂时无法验证会话")
		return
	}
	s.writeSession(w, r, session)
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.sameOrigin(w, r) {
		return
	}
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "application/json" {
		writeErrorText(w, http.StatusUnsupportedMediaType, "请使用 JSON 请求")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	release, ok := s.passwordAttempt(w, r)
	if !ok {
		return
	}
	defer release()
	user, err := s.store.FindUser(r.Context(), auth.NormalizeUsername(body.Username))
	if err != nil && !errors.Is(err, storage.ErrUserNotFound) {
		writeErrorText(w, http.StatusServiceUnavailable, "暂时无法登录")
		return
	}
	valid := auth.VerifyPassword(user.PasswordHash, body.Password)
	if !valid || err != nil || !user.Enabled {
		writeErrorText(w, http.StatusUnauthorized, "账号或密码错误，或账号已被禁用")
		return
	}
	s.authFailures.reset(clientAddress(r))
	s.issueSession(w, r, user)
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.authorize(w, r, false, true) {
		return
	}
	cookie, _ := r.Cookie(sessionCookie)
	if err := s.store.DeleteSession(r.Context(), cookie.Value); err != nil {
		writeErrorText(w, 500, "退出失败，请重试")
		return
	}
	s.setSessionCookie(w, r, "", time.Unix(0, 0))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) authPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.authorize(w, r, false, true) {
		return
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		Password        string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := auth.ValidatePassword(body.Password); err != nil {
		writeError(w, 400, err)
		return
	}
	if body.CurrentPassword == body.Password {
		writeErrorText(w, 400, "新密码不能与当前密码相同")
		return
	}
	release, ok := s.passwordAttempt(w, r)
	if !ok {
		return
	}
	defer release()
	user := requestSession(r).User
	if !auth.VerifyPassword(user.PasswordHash, body.CurrentPassword) {
		writeErrorText(w, http.StatusBadRequest, "当前密码错误")
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeErrorText(w, 500, "密码更新失败")
		return
	}
	if err := s.store.ChangePassword(r.Context(), user, hash); err != nil {
		writeError(w, 409, err)
		return
	}
	user.PasswordHash, user.MustChangePassword = hash, false
	s.authFailures.reset(clientAddress(r))
	s.issueSession(w, r, user)
}

type userInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
	Enabled  bool   `json:"enabled"`
}

func (s *Server) decodeUser(w http.ResponseWriter, r *http.Request, creating bool) (storage.User, bool) {
	var input userInput
	if !decodeJSON(w, r, &input) {
		return storage.User{}, false
	}
	input.Username = auth.NormalizeUsername(input.Username)
	if err := auth.ValidateUsername(input.Username); err != nil {
		writeError(w, 400, err)
		return storage.User{}, false
	}
	if input.Role != "admin" && input.Role != "user" {
		writeErrorText(w, 400, "无效的用户组")
		return storage.User{}, false
	}
	user := storage.User{Username: input.Username, Role: input.Role, Enabled: input.Enabled}
	if creating || input.Password != "" {
		hash, err := auth.HashPassword(input.Password)
		if err != nil {
			writeError(w, 400, err)
			return storage.User{}, false
		}
		user.PasswordHash, user.MustChangePassword = hash, true
	}
	return user, true
}

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		users, err := s.store.ListUsers(r.Context())
		writeResult(w, users, err)
	case http.MethodPost:
		user, ok := s.decodeUser(w, r, true)
		if !ok {
			return
		}
		created, err := s.store.CreateUser(r.Context(), user, false)
		writeResult(w, created, err)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) adminUserItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPut && r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/admin/users/"), 10, 64)
	if err != nil || id <= 0 {
		writeErrorText(w, 400, "无效的用户 ID")
		return
	}
	if id == requestSession(r).User.ID {
		writeErrorText(w, 400, "不能在用户管理中修改或删除当前账号，请使用个人密码设置")
		return
	}
	var update storage.User
	if r.Method == http.MethodPut {
		var ok bool
		update, ok = s.decodeUser(w, r, false)
		if !ok {
			return
		}
	}
	result, err := s.store.UpdateUser(r.Context(), id, update, r.Method == http.MethodDelete)
	writeResult(w, result, err)
}
