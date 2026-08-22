package constraint

import (
	"testing"

	"artifact-dep-resolver/internal/semver"
)

func TestMatch(t *testing.T) {
	mk := func(s string) semver.Version { v, _ := semver.Parse(s); return v }
	cases := []struct {
		raw    string
		ver    string
		expect bool
	}{
		{"1.2.3", "1.2.3", true},
		{"1.2.3", "1.2.4", false},
		{"^1.2.3", "1.9.0", true},
		{"^1.2.3", "2.0.0", false},
		{"^0.2.3", "0.2.9", true},
		{"^0.2.3", "0.3.0", false},
		{"~1.2.3", "1.2.9", true},
		{"~1.2.3", "1.3.0", false},
		{">=1.0.0", "1.0.0", true},
		{">=1.0.0", "0.9.0", false},
		{"<2.0.0", "1.9.0", true},
		{"<2.0.0", "2.0.0", false},
		{">1.0.0 <2.0.0", "1.5.0", true},
		{">1.0.0 <2.0.0", "2.0.0", false},
		{"1.2.3 - 2.0.0", "1.5.0", true},
		{"1.2.3 - 2.0.0", "2.1.0", false},
	}
	for _, c := range cases {
		cons, err := Parse(c.raw)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.raw, err)
		}
		got := cons.Matches(mk(c.ver))
		if got != c.expect {
			t.Fatalf("constraint %q match %q = %v, want %v", c.raw, c.ver, got, c.expect)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	for _, in := range []string{"", "not-a-version", "^^1.0.0", "1.0.0-"} {
		if _, err := Parse(in); err == nil {
			t.Fatalf("Parse(%q) should fail", in)
		}
	}
}

func TestMatchElimination(t *testing.T) {
	mk := func(s string) semver.Version { v, _ := semver.Parse(s); return v }
	cons, err := Parse("^1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if cons.Matches(mk("2.0.0")) {
		t.Fatal("2.0.0 should be eliminated by ^1.0.0")
	}
	if !cons.Matches(mk("1.5.0")) {
		t.Fatal("1.5.0 should match ^1.0.0")
	}
}
