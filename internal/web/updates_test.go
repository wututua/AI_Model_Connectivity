package web

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"cg/internal/update"
)

type updateStub struct {
	starts   int
	resolves int
	err      error
}

const updateRequestID = "0123456789abcdef0123456789abcdef"
const updateStartBody = `{"channel":"stable","version":"v1.0.1","confirm":true,"request_id":"` + updateRequestID + `"}`

type updateRunningAdmin struct{ stubAdmin }

func (updateRunningAdmin) RunningState() RunningState { return RunningState{Running: true} }

func (u *updateStub) Status() (update.Status, error) { return update.Status{Version: "v1.0.0"}, u.err }
func (u *updateStub) Check(context.Context, string) (update.Check, error) {
	return update.Check{}, u.err
}
func (u *updateStub) Resolve(requestID string) (update.Status, error) {
	u.resolves++
	if requestID != updateRequestID {
		return update.Status{}, update.ErrRequest
	}
	return update.Status{Version: "v1.0.0"}, u.err
}
func (u *updateStub) Start(_ context.Context, _, _, requestID string) (update.Job, error) {
	u.starts++
	if requestID != updateRequestID {
		return update.Job{}, update.ErrRequest
	}
	return update.Job{Status: "pending"}, u.err
}

func TestUpdateAPIAuthorizationAndCSRF(t *testing.T) {
	s, _ := newTestServer(t)
	service := &updateStub{}
	s.SetUpdater(service)
	for _, endpoint := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/api/admin/updates", "", 200},
		{"POST", "/api/admin/updates/check", `{"channel":"stable"}`, 200},
		{"POST", "/api/admin/updates/start", updateStartBody, 202},
		{"POST", "/api/admin/updates/resolve", `{"request_id":"` + updateRequestID + `"}`, 200},
	} {
		for role, code := range map[string]int{"": 401, "user": 403, "admin": endpoint.status} {
			rec := perform(s, endpoint.method, endpoint.path, endpoint.body, role)
			if rec.Code != code {
				t.Fatalf("%s %s %s: %d %s", endpoint.method, endpoint.path, role, rec.Code, rec.Body)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("update response cacheable")
			}
		}
		if endpoint.method == "POST" {
			for _, crossSite := range []bool{false, true} {
				req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
				authenticateTestRequest(req, false)
				if crossSite {
					req.Header.Set("Origin", "https://attacker.invalid")
				} else {
					req.Header.Del("X-CSRF-Token")
				}
				rec := httptest.NewRecorder()
				s.Handler().ServeHTTP(rec, req)
				if rec.Code != 403 {
					t.Fatal("CSRF allowed", rec.Code)
				}
			}
		}
	}
	if service.starts != 1 || service.resolves != 1 {
		t.Fatal("unauthorized update mutation", service.starts, service.resolves)
	}
}

func TestUpdateAPIConfirmationMethodsAndErrors(t *testing.T) {
	s, _ := newTestServer(t)
	service := &updateStub{}
	s.SetUpdater(service)
	for _, body := range []string{`{}`, `{"channel":"stable","confirm":false}`, `{"channel":"stable","version":"v1.0.1","confirm":true}`, `{"url":"https://attacker.invalid"}`, `{broken`} {
		if rec := perform(s, "POST", "/api/admin/updates/start", body, "admin"); rec.Code != 400 {
			t.Fatal(rec.Code, rec.Body)
		}
	}
	if service.starts != 0 {
		t.Fatal("unconfirmed update reached worker")
	}
	if rec := perform(s, "GET", "/api/admin/updates/start", "", "admin"); rec.Code != 405 {
		t.Fatal(rec.Code)
	}
	for _, tc := range []struct {
		err  error
		code int
	}{
		{update.ErrBusy, 409}, {update.ErrUnsupported, 409}, {update.ErrRequest, 409}, {update.ErrVersion, 400}, {update.ErrChannel, 400},
		{errors.New("sensitive filesystem path"), 503},
	} {
		service.err = tc.err
		rec := perform(s, "POST", "/api/admin/updates/start", updateStartBody, "admin")
		if rec.Code != tc.code {
			t.Fatal(rec.Code, rec.Body)
		}
	}
}

func TestUpdateCannotStartWhileDetectionRuns(t *testing.T) {
	s, _ := newTestServer(t)
	service := &updateStub{}
	s.SetUpdater(service)
	s.admin = updateRunningAdmin{}
	rec := perform(s, "POST", "/api/admin/updates/start", updateStartBody, "admin")
	if rec.Code != 409 || service.starts != 0 {
		t.Fatal(rec.Code, service.starts)
	}
}

func TestUpdateResolveRequiresCredentialAndDoesNotStartJobs(t *testing.T) {
	s, _ := newTestServer(t)
	service := &updateStub{}
	s.SetUpdater(service)
	for _, body := range []string{`{}`, `{"url":"https://attacker.invalid"}`, `{broken`} {
		if rec := perform(s, "POST", "/api/admin/updates/resolve", body, "admin"); rec.Code != 400 {
			t.Fatal(rec.Code, rec.Body)
		}
	}
	if rec := perform(s, "GET", "/api/admin/updates/resolve", "", "admin"); rec.Code != 405 {
		t.Fatal(rec.Code)
	}
	if rec := perform(s, "POST", "/api/admin/updates/resolve", `{"request_id":"../../unsafe"}`, "admin"); rec.Code != 409 {
		t.Fatal(rec.Code)
	}
	if service.starts != 0 {
		t.Fatal("resolution started an update")
	}
}
