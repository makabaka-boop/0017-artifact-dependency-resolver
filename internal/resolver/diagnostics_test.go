package resolver

import (
	"testing"

	"artifact-dep-resolver/internal/semver"
)

func mustVer(s string) semver.Version { v, _ := semver.Parse(s); return v }

func TestConflictDiagnosticsCapture(t *testing.T) {
	// lib 仅有 1.0.0；a 要求 lib~2.0.0，无任何版本匹配，应产出候选淘汰明细。
	cli := Artifact{
		ID:   1,
		Name: "cli",
		Versions: []ArtifactVersion{{
			ID: 11, Version: "1.0.0", Ver: mustVer("1.0.0"),
			Dependencies: []Dep{{ToArtifactID: 2, ToName: "a", Constraint: mc(">=1.0.0")}},
		}},
	}
	a := Artifact{
		ID:   2,
		Name: "a",
		Versions: []ArtifactVersion{{
			ID: 21, Version: "1.0.0", Ver: mustVer("1.0.0"),
			Dependencies: []Dep{{ToArtifactID: 3, ToName: "lib", Constraint: mc("~2.0.0")}},
		}},
	}
	lib := Artifact{
		ID:   3,
		Name: "lib",
		Versions: []ArtifactVersion{
			mv(31, "1.0.0"),
			mv(32, "1.5.0"),
		},
	}
	ix := NewIndex([]Artifact{cli, a, lib})
	res := Resolve(ix, "cli", nil)
	if res.Status != "failed" {
		t.Fatalf("expected failed, got %s", res.Status)
	}
	found := false
	for _, d := range res.Diagnostics {
		if len(d.Candidates) > 0 {
			found = true
			for _, c := range d.Candidates {
				if c.Artifact == "" || c.Version == "" {
					t.Fatalf("candidate missing fields: %+v", c)
				}
			}
		}
	}
	if !found {
		t.Fatalf("expected candidate diagnostics, got %+v", res.Diagnostics)
	}
}
