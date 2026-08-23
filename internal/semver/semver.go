package semver

import (
	"fmt"
	"strconv"
	"strings"
)

// Version 是一个规范化后的语义化版本。
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	// Build 元数据被保留但参与相等性比较时忽略。
	Build string
	raw   string
}

// Parse 解析并规范化一个 semver 字符串，忽略前导 'v'。
func Parse(s string) (Version, error) {
	in := strings.TrimSpace(s)
	if len(in) > 0 && (in[0] == 'v' || in[0] == 'V') {
		in = in[1:]
	}
	core := in
	build := ""
	hadPlus := false
	if i := strings.IndexByte(in, '+'); i >= 0 {
		hadPlus = true
		core = in[:i]
		build = in[i+1:]
	}
	if hadPlus && build == "" {
		return Version{}, fmt.Errorf("invalid semver %q", s)
	}
	pre := ""
	hadDash := false
	if i := strings.IndexByte(core, '-'); i >= 0 {
		hadDash = true
		pre = core[i+1:]
		core = core[:i]
	}
	if hadDash && pre == "" {
		return Version{}, fmt.Errorf("invalid semver %q", s)
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid semver %q", s)
	}
	major, err1 := parseUint(parts[0])
	minor, err2 := parseUint(parts[1])
	patch, err3 := parseUint(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return Version{}, fmt.Errorf("invalid semver %q", s)
	}
	if pre != "" && !validIdentifiers(pre) {
		return Version{}, fmt.Errorf("invalid prerelease %q", s)
	}
	if build != "" && !validIdentifiers(build) {
		return Version{}, fmt.Errorf("invalid build %q", s)
	}
	return Version{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: pre,
		Build:      build,
		raw:        format(major, minor, patch, pre, build),
	}, nil
}

func parseUint(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, fmt.Errorf("leading zero")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("non-digit")
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func validIdentifiers(s string) bool {
	if s == "" {
		return false
	}
	for _, id := range strings.Split(s, ".") {
		if id == "" {
			return false
		}
		for _, r := range id {
			if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-') {
				return false
			}
		}
	}
	return true
}

// String 返回规范化字符串形式。
func (v Version) String() string { return v.raw }

// Compare 返回 -1、0、1；build 元数据在比较中被忽略。
func (v Version) Compare(o Version) int {
	if v.Major != o.Major {
		return cmpInt(v.Major, o.Major)
	}
	if v.Minor != o.Minor {
		return cmpInt(v.Minor, o.Minor)
	}
	if v.Patch != o.Patch {
		return cmpInt(v.Patch, o.Patch)
	}
	return comparePrerelease(v.Prerelease, o.Prerelease)
}

func comparePrerelease(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if as[i] == bs[i] {
			continue
		}
		// 数值标识符优先级低于字母数字标识符。
		aiNum := isNumeric(as[i])
		biNum := isNumeric(bs[i])
		if aiNum && !biNum {
			return -1
		}
		if !aiNum && biNum {
			return 1
		}
		if aiNum && biNum {
			an, _ := strconv.Atoi(as[i])
			bn, _ := strconv.Atoi(bs[i])
			if an != bn {
				return cmpInt(an, bn)
			}
			continue
		}
		if as[i] < bs[i] {
			return -1
		}
		return 1
	}
	if len(as) < len(bs) {
		return -1
	}
	if len(as) > len(bs) {
		return 1
	}
	return 0
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func format(major, minor, patch int, pre, build string) string {
	s := fmt.Sprintf("%d.%d.%d", major, minor, patch)
	if pre != "" {
		s += "-" + pre
	}
	if build != "" {
		s += "+" + build
	}
	return s
}
