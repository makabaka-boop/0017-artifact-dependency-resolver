package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"artifact-dep-resolver/internal/service"
	"artifact-dep-resolver/internal/store"
)

func newTestServer(t *testing.T) (*Server, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	svc := service.New(st)
	return New(svc), func() { st.Close() }
}

func do(t *testing.T, s http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()
	rec := do(t, s.Handler(), http.MethodGet, "/healthz", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz code = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %q", body["status"])
	}
}

func TestCreateArtifactConflict(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()
	rec := do(t, s.Handler(), http.MethodPost, "/api/v1/artifacts", `{"name":"foo"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create code = %d", rec.Code)
	}
	rec = do(t, s.Handler(), http.MethodPost, "/api/v1/artifacts", `{"name":"foo"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("conflict code = %d", rec.Code)
	}
}

func TestInvalidSemver(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()
	do(t, s.Handler(), http.MethodPost, "/api/v1/artifacts", `{"name":"foo"}`)
	rec := do(t, s.Handler(), http.MethodPost, "/api/v1/artifacts/foo/versions", `{"version":"not-semver"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid semver code = %d", rec.Code)
	}
}

func TestDependencyTargetMissing(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()
	do(t, s.Handler(), http.MethodPost, "/api/v1/artifacts", `{"name":"foo"}`)
	do(t, s.Handler(), http.MethodPost, "/api/v1/artifacts/foo/versions", `{"version":"1.0.0"}`)
	rec := do(t, s.Handler(), http.MethodPut, "/api/v1/artifacts/foo/versions/1.0.0/dependencies",
		`{"dependencies":[{"artifact":"ghost","constraint":"^1.0.0"}]}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("target missing code = %d", rec.Code)
	}
}

func TestErrorEnvelope(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()
	rec := do(t, s.Handler(), http.MethodGet, "/api/v1/artifacts/ghost", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code == "" {
		t.Fatalf("expected error code, got %s", rec.Body.String())
	}
}

func TestEndToEnd(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()
	h := s.Handler()

	if rec := do(t, h, http.MethodPost, "/api/v1/artifacts", `{"name":"cli"}`); rec.Code != 201 {
		t.Fatalf("create cli: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/artifacts", `{"name":"lib"}`); rec.Code != 201 {
		t.Fatalf("create lib: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/artifacts/lib/versions", `{"version":"1.2.0"}`); rec.Code != 201 {
		t.Fatalf("lib 1.2.0: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/artifacts/lib/versions", `{"version":"1.3.0"}`); rec.Code != 201 {
		t.Fatalf("lib 1.3.0: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/artifacts/cli/versions", `{"version":"2.0.0"}`); rec.Code != 201 {
		t.Fatalf("cli 2.0.0: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPut, "/api/v1/artifacts/cli/versions/2.0.0/dependencies",
		`{"dependencies":[{"artifact":"lib","constraint":"^1.0.0"}]}`); rec.Code != 200 {
		t.Fatalf("deps: %d", rec.Code)
	}
	rec := do(t, h, http.MethodPost, "/api/v1/resolve", `{"artifact":"cli"}`)
	if rec.Code != 200 {
		t.Fatalf("resolve: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "1.3.0") {
		t.Fatalf("expected 1.3.0 in resolve result: %s", rec.Body.String())
	}

	// 依赖图。
	rec = do(t, h, http.MethodGet, "/api/v1/artifacts/cli/dependencies", "")
	if rec.Code != 200 {
		t.Fatalf("dep graph: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "lib") {
		t.Fatalf("expected lib in dep graph: %s", rec.Body.String())
	}

	// 历史查询。
	rec = do(t, h, http.MethodGet, "/api/v1/resolutions", "")
	if rec.Code != 200 {
		t.Fatalf("resolutions: %d", rec.Code)
	}
}

func TestResolveFailureReturnsTrackableResolutionID(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()
	h := s.Handler()

	if rec := do(t, h, http.MethodPost, "/api/v1/artifacts", `{"name":"empty"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create empty artifact: %d", rec.Code)
	}

	resolveRec := do(t, h, http.MethodPost, "/api/v1/resolve", `{"artifact":"empty"}`)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("resolve code = %d, body = %s", resolveRec.Code, resolveRec.Body.String())
	}
	var resolveBody struct {
		ResolutionID int64  `json:"resolution_id"`
		Status       string `json:"status"`
	}
	if err := json.Unmarshal(resolveRec.Body.Bytes(), &resolveBody); err != nil {
		t.Fatalf("decode resolve response: %v", err)
	}
	if resolveBody.Status != "failed" {
		t.Fatalf("resolve status = %q, want failed", resolveBody.Status)
	}
	if resolveBody.ResolutionID <= 0 {
		t.Fatalf("resolve resolution_id = %d, want a persisted positive identifier", resolveBody.ResolutionID)
	}

	idPath := "/api/v1/resolutions/" + strconv.FormatInt(resolveBody.ResolutionID, 10)
	detailRec := do(t, h, http.MethodGet, idPath, "")
	if detailRec.Code != http.StatusOK {
		t.Fatalf("resolution detail code = %d, body = %s", detailRec.Code, detailRec.Body.String())
	}
	var detailBody struct {
		Resolution struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"resolution"`
	}
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("decode resolution detail: %v", err)
	}
	if detailBody.Resolution.ID != resolveBody.ResolutionID {
		t.Fatalf("detail resolution id = %d, want %d", detailBody.Resolution.ID, resolveBody.ResolutionID)
	}
	if detailBody.Resolution.Status != "failed" {
		t.Fatalf("detail resolution status = %q, want failed", detailBody.Resolution.Status)
	}

	listRec := do(t, h, http.MethodGet, "/api/v1/resolutions?status=failed", "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("resolution list code = %d, body = %s", listRec.Code, listRec.Body.String())
	}
	var listed []struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode resolution list: %v", err)
	}
	found := false
	for _, item := range listed {
		if item.ID == resolveBody.ResolutionID {
			found = true
			if item.Status != "failed" {
				t.Fatalf("listed resolution status = %q, want failed", item.Status)
			}
			break
		}
	}
	if !found {
		t.Fatalf("resolution list did not contain id %d: %s", resolveBody.ResolutionID, listRec.Body.String())
	}
}
