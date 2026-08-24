package service

import (
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
