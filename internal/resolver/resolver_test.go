package resolver

import (
	"testing"

	"artifact-dep-resolver/internal/constraint"
	"artifact-dep-resolver/internal/semver"
)

func mv(id int64, v string) ArtifactVersion {
	ver, _ := semver.Parse(v)
	return ArtifactVersion{ID: id, Version: v, Ver: ver}
}

func mc(raw string) constraint.Constraint {
	c, _ := constraint.Parse(raw)
	return c
}

func TestSingleHighestVersion(t *testing.T) {
	art := Artifact{
		ID:   1,
		Name: "lib",
		Versions: []ArtifactVersion{
			mv(11, "1.0.0"),
			mv(12, "1.5.0"),
			mv(13, "1.2.0"),
		},
	}
	ix := NewIndex([]Artifact{art})
	res := Resolve(ix, "lib", nil)
	if res.Status != "succeeded" {
		t.Fatalf("status = %s", res.Status)
	}
	if len(res.Resolved) != 1 || res.Resolved[0].Version != "1.5.0" {
		t.Fatalf("resolved = %+v", res.Resolved)
	}
}

func TestTransitiveExpansion(t *testing.T) {
	cli := Artifact{
		ID:   1,
		Name: "cli",
		Versions: []ArtifactVersion{{
			ID:      11,
			Version: "1.0.0",
			Ver:     mustV("1.0.0"),
			Dependencies: []Dep{
				{ToArtifactID: 2, ToName: "lib", Constraint: mc("^1.0.0")},
			},
		}},
	}
	lib := Artifact{
		ID:   2,
		Name: "lib",
		Versions: []ArtifactVersion{
			mv(21, "1.0.0"),
			mv(22, "1.8.0"),
		},
	}
	ix := NewIndex([]Artifact{cli, lib})
	res := Resolve(ix, "cli", nil)
	if res.Status != "succeeded" {
		t.Fatalf("status = %s, diags=%+v", res.Status, res.Diagnostics)
	}
	found := false
	for _, r := range res.Resolved {
		if r.ArtifactName == "lib" && r.Version == "1.8.0" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected lib 1.8.0, got %+v", res.Resolved)
	}
}

func TestConflictBacktrack(t *testing.T) {
	// cli 依赖 a（依赖 lib^1.0.0）与 b（依赖 lib~1.1.x）。
	// lib 有 1.0.0、1.1.0、1.2.0。回溯应选中 1.1.0（同时满足 ^1.0.0 与 ~1.1.0）。
	cli := Artifact{
		ID:   1,
		Name: "cli",
		Versions: []ArtifactVersion{{
			ID:      11,
			Version: "1.0.0",
			Ver:     mustV("1.0.0"),
			Dependencies: []Dep{
				{ToArtifactID: 2, ToName: "a", Constraint: mc(">=1.0.0")},
				{ToArtifactID: 3, ToName: "b", Constraint: mc(">=1.0.0")},
			},
		}},
	}
	a := Artifact{
		ID:   2,
		Name: "a",
		Versions: []ArtifactVersion{{
			ID: 21, Version: "1.0.0", Ver: mustV("1.0.0"),
			Dependencies: []Dep{{ToArtifactID: 4, ToName: "lib", Constraint: mc("^1.0.0")}},
		}},
	}
	b := Artifact{
		ID:   3,
		Name: "b",
		Versions: []ArtifactVersion{{
			ID: 31, Version: "1.0.0", Ver: mustV("1.0.0"),
			Dependencies: []Dep{{ToArtifactID: 4, ToName: "lib", Constraint: mc("~1.1.0")}},
		}},
	}
	lib := Artifact{
		ID:   4,
		Name: "lib",
		Versions: []ArtifactVersion{
			mv(41, "1.0.0"),
			mv(42, "1.1.0"),
			mv(43, "1.2.0"),
		},
	}
	ix := NewIndex([]Artifact{cli, a, b, lib})
	res := Resolve(ix, "cli", nil)
	if res.Status != "succeeded" {
		t.Fatalf("status = %s, diags=%+v", res.Status, res.Diagnostics)
	}
	var libVer string
	for _, r := range res.Resolved {
		if r.ArtifactName == "lib" {
			libVer = r.Version
		}
	}
	if libVer != "1.1.0" {
		t.Fatalf("expected lib 1.1.0, got %q", libVer)
	}
}

func TestCycleDetection(t *testing.T) {
	a := Artifact{
		ID:   1,
		Name: "a",
		Versions: []ArtifactVersion{{
			ID: 11, Version: "1.0.0", Ver: mustV("1.0.0"),
			Dependencies: []Dep{{ToArtifactID: 2, ToName: "b", Constraint: mc(">=1.0.0")}},
		}},
	}
	b := Artifact{
		ID:   2,
		Name: "b",
		Versions: []ArtifactVersion{{
			ID: 21, Version: "1.0.0", Ver: mustV("1.0.0"),
			Dependencies: []Dep{{ToArtifactID: 1, ToName: "a", Constraint: mc(">=1.0.0")}},
		}},
	}
	ix := NewIndex([]Artifact{a, b})
	res := Resolve(ix, "a", nil)
	if res.Status != "failed" {
		t.Fatalf("expected failed, got %s", res.Status)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Type == "CYCLE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected CYCLE diagnostic, got %+v", res.Diagnostics)
	}
}

func TestMissingDependency(t *testing.T) {
	a := Artifact{
		ID:   1,
		Name: "a",
		Versions: []ArtifactVersion{{
			ID: 11, Version: "1.0.0", Ver: mustV("1.0.0"),
			Dependencies: []Dep{{ToArtifactID: 99, ToName: "ghost", Constraint: mc(">=1.0.0")}},
		}},
	}
	ix := NewIndex([]Artifact{a})
	res := Resolve(ix, "a", nil)
	if res.Status != "failed" {
		t.Fatalf("expected failed, got %s", res.Status)
	}
}

func mustV(s string) semver.Version { v, _ := semver.Parse(s); return v }
