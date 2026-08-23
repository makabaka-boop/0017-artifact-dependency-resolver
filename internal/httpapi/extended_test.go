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
