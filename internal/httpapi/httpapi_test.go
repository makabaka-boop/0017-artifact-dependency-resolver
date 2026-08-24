package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
