package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
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

func TestResolvePinnedVersionPersistsMatchingDependency(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()
	h := s.Handler()

	for _, name := range []string{"root", "dep-a", "dep-b", "dep-c"} {
		if rec := do(t, h, http.MethodPost, "/api/v1/artifacts", `{"name":"`+name+`"}`); rec.Code != http.StatusCreated {
			t.Fatalf("create %s: code=%d body=%s", name, rec.Code, rec.Body.String())
		}
	}
	for _, version := range []string{"1.0.0", "2.0.0", "3.0.0"} {
		if rec := do(t, h, http.MethodPost, "/api/v1/artifacts/root/versions", `{"version":"`+version+`"}`); rec.Code != http.StatusCreated {
			t.Fatalf("create root %s: code=%d body=%s", version, rec.Code, rec.Body.String())
		}
	}
	for _, name := range []string{"dep-a", "dep-b", "dep-c"} {
		if rec := do(t, h, http.MethodPost, "/api/v1/artifacts/"+name+"/versions", `{"version":"1.0.0"}`); rec.Code != http.StatusCreated {
			t.Fatalf("create %s version: code=%d body=%s", name, rec.Code, rec.Body.String())
		}
	}
	for version, dep := range map[string]string{"1.0.0": "dep-a", "2.0.0": "dep-b", "3.0.0": "dep-c"} {
		if rec := do(t, h, http.MethodPut, "/api/v1/artifacts/root/versions/"+version+"/dependencies",
			`{"dependencies":[{"artifact":"`+dep+`","constraint":"^1.0.0"}]}`); rec.Code != http.StatusOK {
			t.Fatalf("set %s dependency: code=%d body=%s", version, rec.Code, rec.Body.String())
		}
	}

	rec := do(t, h, http.MethodPost, "/api/v1/resolve", `{"artifact":"root","version":"2.0.0"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("resolve code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resolved struct {
		ResolutionID int64  `json:"resolution_id"`
		Status       string `json:"status"`
		Resolved     []struct {
			Artifact string `json:"artifact"`
			Version  string `json:"version"`
		} `json:"resolved"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resolved); err != nil {
		t.Fatalf("decode resolve response: %v", err)
	}
	if resolved.ResolutionID == 0 {
		t.Fatalf("resolve response missing resolution_id: %s", rec.Body.String())
	}
	if resolved.Status != "succeeded" {
		t.Fatalf("resolve status=%q body=%s", resolved.Status, rec.Body.String())
	}
	if len(resolved.Resolved) != 2 {
		t.Fatalf("resolved=%+v, want root and one dependency", resolved.Resolved)
	}
	foundRoot, foundDepB := false, false
	for _, entry := range resolved.Resolved {
		switch entry.Artifact {
		case "root":
			foundRoot = entry.Version == "2.0.0"
		case "dep-b":
			foundDepB = entry.Version == "1.0.0"
		case "dep-a", "dep-c":
			t.Fatalf("pinned root resolved unexpected dependency: %+v", entry)
		}
	}
	if !foundRoot || !foundDepB {
		t.Fatalf("resolved=%+v, want root@2.0.0 and dep-b@1.0.0", resolved.Resolved)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/resolutions/"+itoa64(resolved.ResolutionID), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("resolution detail code=%d body=%s", rec.Code, rec.Body.String())
	}
	detail := rec.Body.String()
	if !strings.Contains(detail, "dep-b") || !strings.Contains(detail, "1.0.0") {
		t.Fatalf("resolution detail missing dep-b snapshot/item: %s", detail)
	}
	if strings.Contains(detail, "dep-c") {
		t.Fatalf("resolution detail contains wrong dependency dep-c: %s", detail)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/artifacts/root/versions/2.0.0/dependencies", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("dependency list code=%d body=%s", rec.Code, rec.Body.String())
	}
	deps := rec.Body.String()
	if !strings.Contains(deps, "dep-b") || !strings.Contains(deps, "1.0.0") {
		t.Fatalf("root@2.0.0 dependency list missing dep-b: %s", deps)
	}
	if strings.Contains(deps, "dep-a") || strings.Contains(deps, "dep-c") {
		t.Fatalf("root@2.0.0 dependency list contains another version's dependency: %s", deps)
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
