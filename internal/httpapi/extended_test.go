package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestLockfileEndpoint(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()
	h := s.Handler()

	do(t, h, http.MethodPost, "/api/v1/artifacts", `{"name":"cli"}`)
	do(t, h, http.MethodPost, "/api/v1/artifacts", `{"name":"lib"}`)
	do(t, h, http.MethodPost, "/api/v1/artifacts/lib/versions", `{"version":"1.5.0"}`)
	do(t, h, http.MethodPost, "/api/v1/artifacts/cli/versions", `{"version":"2.0.0"}`)
	do(t, h, http.MethodPut, "/api/v1/artifacts/cli/versions/2.0.0/dependencies",
		`{"dependencies":[{"artifact":"lib","constraint":"^1.0.0"}]}`)

	rec := do(t, h, http.MethodPost, "/api/v1/resolve", `{"artifact":"cli"}`)
	if rec.Code != 200 {
		t.Fatalf("resolve code = %d", rec.Code)
	}
	var ro struct {
		ResolutionID int64 `json:"resolution_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ro); err != nil {
		t.Fatal(err)
	}

	rec = do(t, h, http.MethodPost, "/api/v1/lockfiles",
		`{"resolution_id":`+itoa64(ro.ResolutionID)+`,"name":"lck"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create lockfile code = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = do(t, h, http.MethodGet, "/api/v1/lockfiles/lck", "")
	if rec.Code != 200 {
		t.Fatalf("get lockfile code = %d", rec.Code)
	}

	rec = do(t, h, http.MethodPost, "/api/v1/resolve/lockfile",
		`{"lockfile":"lck","artifact":"cli"}`)
	if rec.Code != 200 {
		t.Fatalf("resolve with lockfile code = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDependencyDiffEndpoint(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()
	h := s.Handler()

	do(t, h, http.MethodPost, "/api/v1/artifacts", `{"name":"app"}`)
	do(t, h, http.MethodPost, "/api/v1/artifacts", `{"name":"lib"}`)
	do(t, h, http.MethodPost, "/api/v1/artifacts/app/versions", `{"version":"1.0.0"}`)
	do(t, h, http.MethodPost, "/api/v1/artifacts/app/versions", `{"version":"2.0.0"}`)
	do(t, h, http.MethodPut, "/api/v1/artifacts/app/versions/1.0.0/dependencies",
		`{"dependencies":[{"artifact":"lib","constraint":"^1.0.0"}]}`)
	do(t, h, http.MethodPut, "/api/v1/artifacts/app/versions/2.0.0/dependencies",
		`{"dependencies":[{"artifact":"lib","constraint":"^2.0.0"}]}`)

	rec := do(t, h, http.MethodGet, "/api/v1/artifacts/app/versions/1.0.0/diff/2.0.0", "")
	if rec.Code != 200 {
		t.Fatalf("diff code = %d body=%s", rec.Code, rec.Body.String())
	}
	var diff struct {
		Changed []struct {
			Artifact string `json:"artifact"`
		} `json:"changed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &diff); err != nil {
		t.Fatal(err)
	}
	if len(diff.Changed) != 1 || diff.Changed[0].Artifact != "lib" {
		t.Fatalf("diff changed = %+v", diff.Changed)
	}
}

func TestResolveChoosesDependencyFreeLatestVersionAndPersistsSuccess(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()
	h := s.Handler()

	setup := []struct {
		method string
		path   string
		body   string
		code   int
	}{
		{http.MethodPost, "/api/v1/artifacts", `{"name":"app"}`, http.StatusCreated},
		{http.MethodPost, "/api/v1/artifacts", `{"name":"lib"}`, http.StatusCreated},
		{http.MethodPost, "/api/v1/artifacts/app/versions", `{"version":"2.0.0"}`, http.StatusCreated},
		{http.MethodPost, "/api/v1/artifacts/app/versions", `{"version":"1.0.0"}`, http.StatusCreated},
		{http.MethodPost, "/api/v1/artifacts/lib/versions", `{"version":"1.0.0"}`, http.StatusCreated},
		{
			http.MethodPut,
			"/api/v1/artifacts/app/versions/1.0.0/dependencies",
			`{"dependencies":[{"artifact":"lib","constraint":">=2.0.0"}]}`,
			http.StatusOK,
		},
	}
	for _, req := range setup {
		rec := do(t, h, req.method, req.path, req.body)
		if rec.Code != req.code {
			t.Fatalf("setup %s %s: code = %d, want %d; body=%s", req.method, req.path, rec.Code, req.code, rec.Body.String())
		}
	}

	rec := do(t, h, http.MethodPost, "/api/v1/resolve", `{"artifact":"app"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/resolve: code = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resolved struct {
		ResolutionID int64  `json:"resolution_id"`
		Status       string `json:"status"`
		Resolved     []struct {
			Artifact string `json:"artifact"`
			Version  string `json:"version"`
		} `json:"resolved"`
		Diagnostics []struct {
			Type    string   `json:"type"`
			Message string   `json:"message"`
			Details []string `json:"details"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resolved); err != nil {
		t.Fatalf("decode resolve response: %v; body=%s", err, rec.Body.String())
	}
	if resolved.ResolutionID == 0 {
		t.Errorf("POST /api/v1/resolve returned resolution_id = 0; body=%s", rec.Body.String())
	}
	if resolved.Status != "succeeded" {
		t.Errorf("POST /api/v1/resolve status = %q, want succeeded; diagnostics=%+v", resolved.Status, resolved.Diagnostics)
	}
	if len(resolved.Resolved) != 1 || resolved.Resolved[0].Artifact != "app" || resolved.Resolved[0].Version != "2.0.0" {
		t.Errorf("POST /api/v1/resolve resolved = %+v, want only app 2.0.0", resolved.Resolved)
	}
	if len(resolved.Diagnostics) != 0 {
		t.Errorf("POST /api/v1/resolve diagnostics = %+v, want none", resolved.Diagnostics)
	}

	if resolved.ResolutionID == 0 {
		return
	}
	rec = do(t, h, http.MethodGet, "/api/v1/resolutions/"+itoa64(resolved.ResolutionID), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET resolution history: code = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var history struct {
		Resolution struct {
			Status     string `json:"status"`
			ErrorCode  string `json:"error_code"`
			ResultJSON string `json:"result_json"`
		} `json:"resolution"`
		Items []struct {
			SelectedVersion string `json:"selected_version"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &history); err != nil {
		t.Fatalf("decode resolution history: %v; body=%s", err, rec.Body.String())
	}
	if history.Resolution.Status != "succeeded" {
		t.Errorf("saved resolution status = %q, want succeeded; body=%s", history.Resolution.Status, rec.Body.String())
	}
	if history.Resolution.ErrorCode != "" {
		t.Errorf("saved resolution error_code = %q, want empty", history.Resolution.ErrorCode)
	}
	var resultEntries []struct {
		ArtifactName string `json:"ArtifactName"`
		Version      string `json:"Version"`
	}
	if err := json.Unmarshal([]byte(history.Resolution.ResultJSON), &resultEntries); err != nil {
		t.Errorf("decode saved result_json %q: %v", history.Resolution.ResultJSON, err)
	} else if len(resultEntries) != 1 || resultEntries[0].ArtifactName != "app" || resultEntries[0].Version != "2.0.0" {
		t.Errorf("saved result_json entries = %+v, want only app 2.0.0", resultEntries)
	}
	if len(history.Items) != 1 || history.Items[0].SelectedVersion != "2.0.0" {
		t.Errorf("saved resolution items = %+v, want one item selecting 2.0.0", history.Items)
	}
}

func TestRerunEndpoint(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()
	h := s.Handler()

	do(t, h, http.MethodPost, "/api/v1/artifacts", `{"name":"cli"}`)
	do(t, h, http.MethodPost, "/api/v1/artifacts", `{"name":"lib"}`)
	do(t, h, http.MethodPost, "/api/v1/artifacts/lib/versions", `{"version":"1.0.0"}`)
	do(t, h, http.MethodPost, "/api/v1/artifacts/cli/versions", `{"version":"1.0.0"}`)
	do(t, h, http.MethodPut, "/api/v1/artifacts/cli/versions/1.0.0/dependencies",
		`{"dependencies":[{"artifact":"lib","constraint":"^1.0.0"}]}`)

	rec := do(t, h, http.MethodPost, "/api/v1/resolve", `{"artifact":"cli"}`)
	var ro struct {
		ResolutionID int64 `json:"resolution_id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &ro)

	rec = do(t, h, http.MethodPost, "/api/v1/resolutions/"+itoa64(ro.ResolutionID)+"/rerun", "")
	if rec.Code != 200 {
		t.Fatalf("rerun code = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestReadinessEndpoint(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()
	h := s.Handler()

	do(t, h, http.MethodPost, "/api/v1/artifacts", `{"name":"cli"}`)
	do(t, h, http.MethodPost, "/api/v1/artifacts", `{"name":"lib"}`)
	do(t, h, http.MethodPost, "/api/v1/artifacts/lib/versions", `{"version":"1.0.0"}`)
	do(t, h, http.MethodPost, "/api/v1/artifacts/cli/versions", `{"version":"1.0.0"}`)
	do(t, h, http.MethodPut, "/api/v1/artifacts/cli/versions/1.0.0/dependencies",
		`{"dependencies":[{"artifact":"lib","constraint":"^1.0.0"}]}`)

	rec := do(t, h, http.MethodGet, "/api/v1/artifacts/cli/versions/1.0.0/readiness", "")
	if rec.Code != 200 {
		t.Fatalf("readiness code = %d body=%s", rec.Code, rec.Body.String())
	}
	var report struct {
		Ready bool `json:"ready"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Ready {
		t.Fatalf("expected ready, got %s", rec.Body.String())
	}
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
