package update

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

const (
	Repository = "wututua/AI_Model_Connectivity"
	StateDir   = "/var/lib/model-connectivity-updater"
	HelperDir  = "/usr/local/lib/model-connectivity-updater"
	InstallDir = "/opt/model-connectivity"
)

var (
	ErrUnsupported = errors.New("此部署尚未启用一键更新")
	ErrBusy        = errors.New("已有更新任务，未确认结果前不能再次更新")
	ErrVersion     = errors.New("目标版本不是当前通道中已检查的新版本，请重新检查")
	ErrChannel     = errors.New("更新通道必须为 stable 或 preview")
	ErrRequest     = errors.New("更新提交凭据已失效，请刷新状态后重新确认")
	tagPattern     = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$`)
	idPattern      = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

func validTag(tag string) bool         { return tagPattern.MatchString(tag) && semver.IsValid(tag) }
func validChannel(channel string) bool { return channel == "stable" || channel == "preview" }

type Release struct {
	Version          string    `json:"version"`
	Notes            string    `json:"notes"`
	URL              string    `json:"url"`
	PublishedAt      time.Time `json:"published_at"`
	Prerelease       bool      `json:"prerelease"`
	PackageAvailable bool      `json:"package_available"`
}

type Check struct {
	Channel   string    `json:"channel"`
	CheckedAt time.Time `json:"checked_at"`
	Available bool      `json:"available"`
	Release   *Release  `json:"release"`
}

type Job struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	Channel   string    `json:"channel"`
	Status    string    `json:"status"`
	Stage     string    `json:"stage"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Message   string    `json:"message"`
}

func (j Job) Active() bool {
	return j.Status == "pending" || j.Status == "running" || j.Status == "recovery_required"
}

type Status struct {
	RequestID  string `json:"request_id"`
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	Platform   string `json:"platform"`
	Deployment string `json:"deployment"`
	Supported  bool   `json:"supported"`
	Reason     string `json:"reason"`
	Job        *Job   `json:"job"`
}

type Manager struct {
	version, commit, platform, deployment, dir string
	client                                     *http.Client
	api                                        string
	checkMu                                    sync.Mutex
	submitMu                                   sync.Mutex
	cache                                      map[string]Check
	supported                                  func() bool
	requestID                                  string
}

func New(version, commit string) *Manager {
	deployment := "manual"
	if _, err := os.Stat("/.dockerenv"); err == nil || os.Getenv("CG_DEPLOYMENT") == "docker" {
		deployment = "docker"
	}
	m := &Manager{version: version, commit: commit, platform: runtime.GOOS + "/" + runtime.GOARCH,
		deployment: deployment, dir: StateDir, api: "https://api.github.com/repos/" + Repository,
		client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}}, cache: make(map[string]Check)}
	m.supported = func() bool {
		executable, err := os.Executable()
		if err != nil || deployment == "docker" || !validTag(version) ||
			executable != InstallDir+"/model-connectivity" || runtime.GOOS != "linux" {
			return false
		}
		for _, path := range []string{StateDir, HelperDir, HelperDir + "/model-connectivity", HelperDir + "/install.sh", StateDir + "/enabled"} {
			if !trustedPath(path) {
				return false
			}
		}
		data, err := os.ReadFile(StateDir + "/enabled")
		return err == nil && string(data) == "1\n"
	}
	return m
}

func (m *Manager) Status() (Status, error) {
	m.submitMu.Lock()
	defer m.submitMu.Unlock()
	return m.status()
}

func (m *Manager) renewRequestID() error {
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return err
	}
	m.requestID = hex.EncodeToString(token)
	return nil
}

// The caller holds submitMu so the credential and job belong to one snapshot.
func (m *Manager) status() (Status, error) {
	if m.requestID == "" {
		if err := m.renewRequestID(); err != nil {
			return Status{}, err
		}
	}
	s := Status{RequestID: m.requestID, Version: m.version, Commit: m.commit, Platform: m.platform,
		Deployment: m.deployment, Supported: m.supported()}
	if s.Supported {
		s.Deployment = "systemd"
		s.Reason = ""
	} else if m.deployment == "docker" {
		s.Reason = "Docker 部署请在宿主机更新镜像并重建容器，保留数据卷。"
	} else if !validTag(m.version) {
		s.Reason = "开发构建只能检查发布版本，不能一键更新。请先安装官方发布包。"
	} else {
		s.Reason = "一键更新仅支持已启用更新服务的 Linux 脚本安装。其他部署请下载完整发布包手动更新。"
	}
	// Only the fixed, installer-owned state directory is exposed, never arbitrary files.
	if !s.Supported && !trustedPath(m.dir) {
		return s, nil
	}
	job, err := m.readJob()
	if err != nil {
		return s, err
	}
	s.Job = job
	return s, nil
}

// Resolve closes an uncertain submission without replaying it. A delayed Start
// cannot enqueue after this returns; credentials also expire on process restart.
func (m *Manager) Resolve(requestID string) (Status, error) {
	if !idPattern.MatchString(requestID) {
		return Status{}, ErrRequest
	}
	m.submitMu.Lock()
	defer m.submitMu.Unlock()
	if requestID == m.requestID {
		if err := m.renewRequestID(); err != nil {
			return Status{}, err
		}
	}
	return m.status()
}

func (m *Manager) readJob() (*Job, error) {
	return readJobFiles(m.dir, readJSON)
}

func readJobFiles(dir string, read func(string, any, int64) error) (*Job, error) {
	var status Job
	statusPath := filepath.Join(dir, "status.json")
	err := read(statusPath, &status, 8192)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil && status.Active() {
		return &status, nil
	}
	var pending Job
	err = read(filepath.Join(dir, "requests", "request.json"), &pending, 4096)
	if err == nil {
		pending.Status = "pending"
		pending.Stage = "queued"
		return &pending, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	// The worker may have published running and removed the inbox between reads.
	// Its status is durable before removal, so an absent inbox requires a reread.
	status = Job{}
	if err := read(statusPath, &status, 8192); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if status.ID != "" {
		return &status, nil
	}
	return nil, nil
}

func (m *Manager) Busy() bool {
	if !m.supported() {
		return false
	}
	job, err := m.readJob()
	return err != nil || job != nil && job.Active()
}

func (m *Manager) Check(ctx context.Context, channel string) (Check, error) {
	if !validChannel(channel) {
		return Check{}, ErrChannel
	}
	m.checkMu.Lock()
	defer m.checkMu.Unlock()
	if cached, ok := m.cache[channel]; ok && time.Since(cached.CheckedAt) < 5*time.Minute {
		return cached, nil
	}
	path := "/releases/latest"
	if channel == "preview" {
		path = "/releases?per_page=100"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.api+path, nil)
	if err != nil {
		return Check{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "AI-Model-Connectivity-Updater")
	res, err := m.client.Do(req)
	if err != nil {
		return Check{}, fmt.Errorf("无法连接 GitHub，请稍后重试")
	}
	defer res.Body.Close()
	checked := Check{Channel: channel, CheckedAt: time.Now().UTC()}
	if res.StatusCode == http.StatusNotFound && channel == "stable" {
		m.cache[channel] = checked
		return checked, nil
	}
	if res.StatusCode != http.StatusOK {
		return Check{}, fmt.Errorf("GitHub 返回 HTTP %d，请稍后重试", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4*1024*1024+1))
	if err != nil || len(body) > 4*1024*1024 {
		return Check{}, errors.New("发布信息读取失败或过大")
	}
	var candidates []githubRelease
	if channel == "stable" {
		var release githubRelease
		if err := json.Unmarshal(body, &release); err != nil {
			return Check{}, errors.New("发布信息格式错误")
		}
		candidates = append(candidates, release)
	} else if err := json.Unmarshal(body, &candidates); err != nil {
		return Check{}, errors.New("发布信息格式错误")
	}
	for _, r := range candidates {
		if r.Draft || !validTag(r.Tag) || channel == "stable" && (r.Prerelease || semver.Prerelease(r.Tag) != "") {
			continue
		}
		if checked.Release != nil && !r.PublishedAt.After(checked.Release.PublishedAt) {
			continue
		}
		asset := assetName(m.platform)
		hasBinary, hasChecksum := false, false
		for _, a := range r.Assets {
			if a.State != "uploaded" {
				continue
			}
			if a.Name == asset {
				hasBinary = true
			}
			if a.Name == "SHA256SUMS.txt" {
				hasChecksum = true
			}
		}
		checked.Release = &Release{Version: r.Tag, Notes: r.Body, PublishedAt: r.PublishedAt,
			Prerelease: r.Prerelease || semver.Prerelease(r.Tag) != "",
			URL:        "https://github.com/" + Repository + "/releases/tag/" + r.Tag, PackageAvailable: hasBinary && hasChecksum}
	}
	checked.Available = checked.Release != nil && validTag(m.version) && semver.Compare(checked.Release.Version, m.version) > 0
	m.cache[channel] = checked
	return checked, nil
}

type githubRelease struct {
	Tag         string    `json:"tag_name"`
	Body        string    `json:"body"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"assets"`
}

func assetName(platform string) string {
	ext := ".tar.gz"
	if strings.HasPrefix(platform, "windows/") {
		ext = ".zip"
	}
	return "model-connectivity-" + strings.ReplaceAll(platform, "/", "-") + ext
}

func (m *Manager) Start(ctx context.Context, channel, version, requestID string) (Job, error) {
	if !validChannel(channel) {
		return Job{}, ErrChannel
	}
	if !validTag(version) {
		return Job{}, ErrVersion
	}
	if !m.supported() {
		return Job{}, ErrUnsupported
	}
	if !idPattern.MatchString(requestID) {
		return Job{}, ErrRequest
	}
	checked, err := m.Check(ctx, channel)
	if err != nil {
		return Job{}, err
	}
	if !checked.Available || checked.Release == nil || !checked.Release.PackageAvailable || checked.Release.Version != version {
		return Job{}, ErrVersion
	}
	m.submitMu.Lock()
	defer m.submitMu.Unlock()
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}
	if requestID != m.requestID {
		return Job{}, ErrRequest
	}
	if !m.supported() {
		return Job{}, ErrUnsupported
	}
	if m.Busy() {
		return Job{}, ErrBusy
	}
	// Consume before writing: even an ambiguous rename/fsync error cannot replay
	// the same request. Resolve inspects the queue/status under this same lock.
	if err := m.renewRequestID(); err != nil {
		return Job{}, err
	}
	now := time.Now().UTC()
	job := Job{ID: requestID, Version: version, Channel: channel,
		Status: "pending", Stage: "queued", CreatedAt: now, UpdatedAt: now, Message: "已提交，等待独立更新服务"}
	if err := atomicJSON(filepath.Join(m.dir, "requests", "request.json"), job, 0600); err != nil {
		return Job{}, err
	}
	return job, nil
}

func readJSON(path string, value any, limit int64) error {
	f, err := openRegular(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return errors.New("update file too large")
	}
	return json.Unmarshal(data, value)
}

func atomicJSON(path string, value any, mode os.FileMode) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".update-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(append(data, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
