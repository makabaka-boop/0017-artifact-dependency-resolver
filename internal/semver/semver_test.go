package semver

import "testing"

func TestParseValid(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantPre string
	}{
		{"1.2.3", "1.2.3", ""},
		{"v1.2.3", "1.2.3", ""},
		{"1.2.3-alpha.1", "1.2.3-alpha.1", "alpha.1"},
		{"1.2.3+build.5", "1.2.3+build.5", ""},
		{"1.2.3-alpha+build", "1.2.3-alpha+build", "alpha"},
	}
	for _, c := range cases {
		v, err := Parse(c.in)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.in, err)
		}
		if v.String() != c.want {
			t.Fatalf("Parse(%q) = %q, want %q", c.in, v.String(), c.want)
		}
		if v.Prerelease != c.wantPre {
			t.Fatalf("Parse(%q) prerelease = %q, want %q", c.in, v.Prerelease, c.wantPre)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	for _, in := range []string{"", "1.2", "1.2.3.4", "x.y.z", "1.2.3-", "1.2.3+", "01.2.3"} {
		if _, err := Parse(in); err == nil {
			t.Fatalf("Parse(%q) should fail", in)
		}
	}
}

func TestCompare(t *testing.T) {
	ordered := []string{
		"1.0.0-alpha",
		"1.0.0-alpha.1",
		"1.0.0-alpha.beta",
		"1.0.0-beta",
		"1.0.0-beta.2",
		"1.0.0-beta.11",
		"1.0.0-rc.1",
		"1.0.0",
		"2.0.0",
		"2.1.0",
		"2.1.1",
	}
	for i := 1; i < len(ordered); i++ {
		a, _ := Parse(ordered[i-1])
		b, _ := Parse(ordered[i])
		if a.Compare(b) >= 0 {
			t.Fatalf("%s should be < %s", ordered[i-1], ordered[i])
		}
	}
	// build 元数据被忽略。
	a, _ := Parse("1.0.0+x")
	b, _ := Parse("1.0.0+y")
	if a.Compare(b) != 0 {
		t.Fatalf("build metadata should be ignored in comparison")
	}
}
