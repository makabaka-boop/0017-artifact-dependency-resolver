package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"artifact-dep-resolver/internal/store"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st)
}

func seedArtifactWithDeps(t *testing.T, s *Service) {
	t.Helper()
	if _, err := s.CreateArtifact("cli"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateArtifact("lib"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("lib", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("lib", "1.5.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("cli", "2.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDependencies("cli", "2.0.0", []DependencyInput{
		{Artifact: "lib", Constraint: "^1.0.0"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCreateLockfileAndResolveWith(t *testing.T) {
	s := newTestService(t)
	seedArtifactWithDeps(t, s)

	out, err := s.Resolve("cli", nil)
	if err != nil {
		t.Fatal(err)
	}
	lf, err := s.CreateLockfile(out.ResolutionID, "mylock")
	if err != nil {
		t.Fatal(err)
	}
	if lf.ID == 0 || lf.Name != "mylock" {
		t.Fatalf("lockfile = %+v", lf)
	}
	if len(lf.Artifacts) != 2 {
		t.Fatalf("expected 2 pins, got %d", len(lf.Artifacts))
	}

	got, err := s.GetLockfile("mylock")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != lf.ID {
		t.Fatalf("get lockfile id mismatch")
	}

	verify, err := s.ResolveWithLockfile("mylock", "cli", nil)
	if err != nil {
		t.Fatal(err)
	}
	if verify.Status != "in_sync" {
		t.Fatalf("expected in_sync, got %s", verify.Status)
	}
}

func TestResolveWithLockfileDetectsDependencyDrift(t *testing.T) {
	s := newTestService(t)
	if _, err := s.CreateArtifact("app"); err != nil {
		t.Fatal(err)
	}
	lib, err := s.CreateArtifact("lib")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("app", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("lib", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDependencies("app", "1.0.0", []DependencyInput{
		{Artifact: "lib", Constraint: ">=1.0.0"},
	}); err != nil {
		t.Fatal(err)
	}

	rootVersion := "1.0.0"
	baselineResolution, err := s.Resolve("app", &rootVersion)
	if err != nil {
		t.Fatal(err)
	}
	if baselineResolution.Status != "succeeded" {
		t.Fatalf("baseline status = %s", baselineResolution.Status)
	}
	baselineLock, err := s.CreateLockfile(baselineResolution.ResolutionID, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	if !hasLockPin(baselineLock.Artifacts, "app", "1.0.0") || !hasLockPin(baselineLock.Artifacts, "lib", "1.0.0") {
		t.Fatalf("baseline lockfile missing lib@1.0.0: %+v", baselineLock.Artifacts)
	}

	for _, version := range []string{"1.1.0", "2.0.0"} {
		if _, err := s.CreateVersion("lib", version); err != nil {
			t.Fatal(err)
		}
	}

	verify, err := s.ResolveWithLockfile("baseline", "app", &rootVersion)
	if err != nil {
		t.Fatal(err)
	}
	if verify.Status != "drifted" {
		t.Fatalf("expected drifted status, got %s", verify.Status)
	}
	if verify.Lockfile != "baseline" {
		t.Fatalf("lockfile = %q", verify.Lockfile)
	}
	if got := resolvedVersion(verify.Resolved, "app"); got != "1.0.0" {
		t.Fatalf("expected resolved app@1.0.0, got %q", got)
	}
	if got := resolvedVersion(verify.Resolved, "lib"); got != "2.0.0" {
		t.Fatalf("expected resolved lib@2.0.0, got %q", got)
	}
	if len(verify.Drifted) != 1 || verify.Drifted[0] != (DriftEntry{Artifact: "lib", Locked: "1.0.0", Resolved: "2.0.0"}) {
		t.Fatalf("drift = %+v", verify.Drifted)
	}

	unchangedLock, err := s.GetLockfile("baseline")
	if err != nil {
		t.Fatal(err)
	}
	if !hasLockPin(unchangedLock.Artifacts, "app", "1.0.0") || !hasLockPin(unchangedLock.Artifacts, "lib", "1.0.0") {
		t.Fatalf("lockfile pin changed: %+v", unchangedLock.Artifacts)
	}

	items, err := s.store.ListResolutionItems(verify.ResolutionID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.ArtifactID == lib.ID {
			found = true
			if item.SelectedVersion != "2.0.0" {
				t.Fatalf("persisted lib version = %s", item.SelectedVersion)
			}
		}
	}
	if !found {
		t.Fatalf("persisted resolution missing lib item: %+v", items)
	}
}

func TestResolveLockfileEndpointDetectsDependencyDrift(t *testing.T) {
	s := newTestService(t)
	if _, err := s.CreateArtifact("app"); err != nil {
		t.Fatal(err)
	}
	lib, err := s.CreateArtifact("lib")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("app", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("lib", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDependencies("app", "1.0.0", []DependencyInput{
		{Artifact: "lib", Constraint: ">=1.0.0"},
	}); err != nil {
		t.Fatal(err)
	}

	rootVersion := "1.0.0"
	baseline, err := s.Resolve("app", &rootVersion)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Status != "succeeded" {
		t.Fatalf("baseline status = %s", baseline.Status)
	}
	if _, err := s.CreateLockfile(baseline.ResolutionID, "baseline"); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"1.1.0", "2.0.0"} {
		if _, err := s.CreateVersion("lib", version); err != nil {
			t.Fatal(err)
		}
	}

	payload, err := json.Marshal(map[string]string{
		"lockfile": "baseline",
		"artifact": "app",
		"version":  "1.0.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/resolve/lockfile", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/resolve/lockfile" {
			http.NotFound(w, r)
			return
		}
		var in struct {
			Lockfile string `json:"lockfile"`
			Artifact string `json:"artifact"`
			Version  string `json:"version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		verify, err := s.ResolveWithLockfile(in.Lockfile, in.Artifact, &in.Version)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(verify)
	}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var verify LockfileVerifyOutput
	if err := json.NewDecoder(rec.Body).Decode(&verify); err != nil {
		t.Fatal(err)
	}
	if verify.Status != "drifted" {
		t.Fatalf("expected drifted status, got %s", verify.Status)
	}
	if got := resolvedVersion(verify.Resolved, "lib"); got != "2.0.0" {
		t.Fatalf("expected resolved lib@2.0.0, got %q", got)
	}
	if len(verify.Drifted) != 1 || verify.Drifted[0] != (DriftEntry{Artifact: "lib", Locked: "1.0.0", Resolved: "2.0.0"}) {
		t.Fatalf("drift = %+v", verify.Drifted)
	}
	if verify.ResolutionID == 0 {
		t.Fatal("expected resolution id")
	}

	lock, err := s.GetLockfile("baseline")
	if err != nil {
		t.Fatal(err)
	}
	if !hasLockPin(lock.Artifacts, "app", "1.0.0") || !hasLockPin(lock.Artifacts, "lib", "1.0.0") || hasLockPin(lock.Artifacts, "lib", "2.0.0") {
		t.Fatalf("lockfile pins = %+v", lock.Artifacts)
	}
	items, err := s.store.ListResolutionItems(verify.ResolutionID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.ArtifactID == lib.ID {
			found = true
			if item.SelectedVersion != "2.0.0" {
				t.Fatalf("persisted lib version = %s", item.SelectedVersion)
			}
		}
	}
	if !found {
		t.Fatalf("persisted resolution missing lib item: %+v", items)
	}
}

func TestPOSTResolveLockfileReportsDriftAndPersistsLatestResolution(t *testing.T) {
	s := newTestService(t)

	lib, err := s.CreateArtifact("lib")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("lib", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateArtifact("app"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("app", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDependencies("app", "1.0.0", []DependencyInput{
		{Artifact: "lib", Constraint: ">=1.0.0"},
	}); err != nil {
		t.Fatal(err)
	}

	appVersion := "1.0.0"
	baseline, err := s.Resolve("app", &appVersion)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Status != "succeeded" {
		t.Fatalf("baseline status = %q, resolved = %+v", baseline.Status, baseline.Resolved)
	}
	if got := resolvedVersion(baseline.Resolved, "lib"); got != "1.0.0" {
		t.Fatalf("baseline lib version = %q, resolved = %+v", got, baseline.Resolved)
	}
	lockfile, err := s.CreateLockfile(baseline.ResolutionID, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	if got := pinnedVersion(lockfile.Artifacts, "lib"); got != "1.0.0" {
		t.Fatalf("baseline lockfile lib version = %q, artifacts = %+v", got, lockfile.Artifacts)
	}

	if _, err := s.CreateVersion("lib", "1.1.0"); err != nil {
		t.Fatal(err)
	}
	latestLib, err := s.CreateVersion("lib", "2.0.0")
	if err != nil {
		t.Fatal(err)
	}

	const endpoint = "/api/v1/resolve/lockfile"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != endpoint {
			http.NotFound(w, r)
			return
		}
		var input struct {
			Lockfile string `json:"lockfile"`
			Artifact string `json:"artifact"`
			Version  string `json:"version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		out, err := s.ResolveWithLockfile(input.Lockfile, input.Artifact, &input.Version)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(out); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})

	req := httptest.NewRequest(http.MethodPost, endpoint, bytes.NewBufferString(
		`{"lockfile":"baseline","artifact":"app","version":"1.0.0"}`,
	))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	responseBody := recorder.Body.String()
	t.Logf("POST %s status=%d body=%s", endpoint, recorder.Code, responseBody)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST %s status = %d, want %d; body=%s", endpoint, recorder.Code, http.StatusOK, responseBody)
	}
	var verify LockfileVerifyOutput
	if err := json.Unmarshal([]byte(responseBody), &verify); err != nil {
		t.Fatalf("decode POST %s response: %v; body=%s", endpoint, err, responseBody)
	}
	if verify.ResolutionID == 0 {
		t.Fatalf("POST %s resolution_id = 0; body=%s", endpoint, responseBody)
	}
	persistedResolution, err := s.store.GetResolution(verify.ResolutionID)
	if err != nil {
		t.Fatalf("query resolution %d: %v", verify.ResolutionID, err)
	}
	persistedItems, err := s.store.ListResolutionItems(verify.ResolutionID)
	if err != nil {
		t.Fatalf("query resolution items %d: %v", verify.ResolutionID, err)
	}
	t.Logf("lockfile resolution output: %+v", verify)
	t.Logf("persisted resolution: %+v", persistedResolution)
	t.Logf("persisted resolution items: %+v", persistedItems)

	if verify.Status != "drifted" {
		t.Errorf("status = %q, want drifted", verify.Status)
	}
	if got := resolvedVersion(verify.Resolved, "app"); got != "1.0.0" {
		t.Errorf("resolved app version = %q, want 1.0.0", got)
	}
	if got := resolvedVersion(verify.Resolved, "lib"); got != "2.0.0" {
		t.Errorf("resolved lib version = %q, want 2.0.0", got)
	}
	if len(verify.Drifted) != 1 {
		t.Errorf("drifted = %+v, want exactly one entry", verify.Drifted)
	} else {
		drift := verify.Drifted[0]
		if drift.Artifact != "lib" || drift.Locked != "1.0.0" || drift.Resolved != "2.0.0" {
			t.Errorf("drifted[0] = %+v, want lib locked=1.0.0 resolved=2.0.0", drift)
		}
	}
	if persistedResolution.ID != verify.ResolutionID {
		t.Errorf("persisted resolution id = %d, want %d", persistedResolution.ID, verify.ResolutionID)
	}
	if persistedResolution.Status != "succeeded" {
		t.Errorf("persisted resolution status = %q, want succeeded", persistedResolution.Status)
	}
	foundPersistedLib := false
	for _, item := range persistedItems {
		if item.ArtifactID != lib.ID {
			continue
		}
		foundPersistedLib = true
		if item.VersionID != latestLib.ID || item.SelectedVersion != "2.0.0" {
			t.Errorf("persisted lib item = %+v, want version_id=%d selected_version=2.0.0", item, latestLib.ID)
		}
	}
	if !foundPersistedLib {
		t.Errorf("persisted resolution items have no lib entry: %+v", persistedItems)
	}
}

func resolvedVersion(entries []ResolvedEntry, artifact string) string {
	for _, entry := range entries {
		if entry.Artifact == artifact {
			return entry.Version
		}
	}
	return ""
}

func hasLockPin(entries []LockfilePinEntry, artifact, version string) bool {
	for _, entry := range entries {
		if entry.Artifact == artifact && entry.Version == version {
			return true
		}
	}
	return false
}

func pinnedVersion(entries []LockfilePinEntry, artifact string) string {
	for _, entry := range entries {
		if entry.Artifact == artifact {
			return entry.Version
		}
	}
	return ""
}

func TestCreateLockfileFailedResolution(t *testing.T) {
	s := newTestService(t)
	// 制造一个无解的解析（环），并验证无法对其生成锁定。
	if _, err := s.CreateArtifact("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateArtifact("b"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("a", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("b", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDependencies("a", "1.0.0", []DependencyInput{
		{Artifact: "b", Constraint: ">=1.0.0"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDependencies("b", "1.0.0", []DependencyInput{
		{Artifact: "a", Constraint: ">=1.0.0"},
	}); err != nil {
		t.Fatal(err)
	}
	out, err := s.Resolve("a", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "failed" {
		t.Fatal("expected failed resolution from cycle")
	}
	if _, err := s.CreateLockfile(out.ResolutionID, "badlock"); err == nil {
		t.Fatal("expected error when locking failed resolution")
	}
}

func TestCreateLockfileDuplicateName(t *testing.T) {
	s := newTestService(t)
	seedArtifactWithDeps(t, s)
	out, _ := s.Resolve("cli", nil)
	if _, err := s.CreateLockfile(out.ResolutionID, "dup"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateLockfile(out.ResolutionID, "dup"); err == nil {
		t.Fatal("expected duplicate lockfile name error")
	}
}

func TestDiffDependencies(t *testing.T) {
	s := newTestService(t)
	if _, err := s.CreateArtifact("app"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateArtifact("lib"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("app", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("app", "2.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDependencies("app", "1.0.0", []DependencyInput{
		{Artifact: "lib", Constraint: "^1.0.0"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDependencies("app", "2.0.0", []DependencyInput{
		{Artifact: "lib", Constraint: "^2.0.0"},
	}); err != nil {
		t.Fatal(err)
	}
	diff, err := s.DiffDependencies("app", "1.0.0", "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Changed) != 1 || diff.Changed[0].Artifact != "lib" {
		t.Fatalf("diff changed = %+v", diff.Changed)
	}
	if diff.Changed[0].ConstraintOld != "^1.0.0" || diff.Changed[0].ConstraintNew != "^2.0.0" {
		t.Fatalf("diff constraint = %+v", diff.Changed[0])
	}
}

func TestRerunResolution(t *testing.T) {
	s := newTestService(t)
	seedArtifactWithDeps(t, s)
	out, err := s.Resolve("cli", nil)
	if err != nil {
		t.Fatal(err)
	}
	rerun, err := s.RerunResolution(out.ResolutionID)
	if err != nil {
		t.Fatal(err)
	}
	if rerun.Status != "succeeded" {
		t.Fatalf("rerun status = %s", rerun.Status)
	}
	if rerun.ResolutionID == out.ResolutionID {
		t.Fatal("rerun should produce a new resolution id")
	}
}

func TestCheckReadiness(t *testing.T) {
	s := newTestService(t)
	seedArtifactWithDeps(t, s)
	report, err := s.CheckReadiness("cli", "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !report.Ready {
		t.Fatalf("expected ready, got blockers=%+v", report.Blockers)
	}
}

func TestDependencyGraphCycleAnnotation(t *testing.T) {
	s := newTestService(t)
	if _, err := s.CreateArtifact("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateArtifact("b"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("a", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("b", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDependencies("a", "1.0.0", []DependencyInput{
		{Artifact: "b", Constraint: ">=1.0.0"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDependencies("b", "1.0.0", []DependencyInput{
		{Artifact: "a", Constraint: ">=1.0.0"},
	}); err != nil {
		t.Fatal(err)
	}
	g, err := s.DependencyGraph("a", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Cycles) == 0 {
		t.Fatalf("expected cycle annotation, got %+v", g)
	}
}

func TestCheckReadinessBlocked(t *testing.T) {
	s := newTestService(t)
	// 制造环：a 依赖 b，b 依赖 a。
	if _, err := s.CreateArtifact("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateArtifact("b"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("a", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion("b", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDependencies("a", "1.0.0", []DependencyInput{
		{Artifact: "b", Constraint: ">=1.0.0"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDependencies("b", "1.0.0", []DependencyInput{
		{Artifact: "a", Constraint: ">=1.0.0"},
	}); err != nil {
		t.Fatal(err)
	}
	report, err := s.CheckReadiness("a", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if report.Ready {
		t.Fatal("expected not ready due to cycle")
	}
	if len(report.Blockers) == 0 {
		t.Fatal("expected blockers")
	}
}
