package constraint

import (
	"fmt"
	"strings"

	"artifact-dep-resolver/internal/semver"
)

// Node 表示一个单一约束原子（比较、^、~ 或区间）。
type Node struct {
	op    string
	ver   semver.Version // 用于比较符
	lower semver.Version // 用于区间与 ^ ~
	upper semver.Version // 用于区间
	inclL bool
	inclU bool
}

// Constraint 表示多个原子的 AND 组合。
type Constraint struct {
	nodes []Node
	raw   string
}

// Parse 解析约束原文，支持操作符、^、~ 以及以逗号或空格分隔的 AND 区间。
func Parse(raw string) (Constraint, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Constraint{}, fmt.Errorf("empty constraint")
	}
	c, rest, err := parseSub(s)
	if err != nil {
		return Constraint{}, err
	}
	rest = strings.TrimSpace(rest)
	if rest != "" && rest[0] == ',' {
		rest = strings.TrimSpace(rest[1:])
	}
	// 剩余部分若还有内容，按 AND 继续解析；支持空格分隔与逗号分隔。
	for rest != "" {
		if rest[0] == ',' {
			rest = strings.TrimSpace(rest[1:])
		}
		if rest == "" {
			break
		}
		sub, r2, err := parseSub(rest)
		if err != nil {
			return Constraint{}, err
		}
		c.nodes = append(c.nodes, sub.nodes...)
		rest = strings.TrimSpace(r2)
		if strings.HasPrefix(rest, ",") {
			rest = strings.TrimSpace(rest[1:])
		}
	}
	c.raw = raw
	return c, nil
}

func parseSub(s string) (Constraint, string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Constraint{}, "", fmt.Errorf("empty clause")
	}
	// 连字符区间：1.2.3 - 2.0.0
	if i := strings.Index(s, "-"); i > 0 {
		// 排除 '-1.0.0' 这种比较写法与 pre 里的 '-'，这里简化为空白分隔的连字符区间。
		left := strings.TrimSpace(s[:i])
		if !strings.HasPrefix(left, ">") && !strings.HasPrefix(left, "<") && !strings.HasPrefix(left, "=") {
			rl := strings.TrimSpace(s[i+1:])
			lo, err := semver.Parse(left)
			if err == nil {
				var hi semver.Version
				rest := ""
				if sp := strings.IndexAny(rl, " ,"); sp >= 0 {
					hi, err = semver.Parse(strings.TrimSpace(rl[:sp]))
					rest = rl[sp:]
				} else {
					hi, err = semver.Parse(strings.TrimSpace(rl))
				}
				if err == nil {
					return Constraint{nodes: []Node{{op: "range", lower: lo, upper: hi, inclL: true, inclU: true}}}, rest, nil
				}
			}
		}
	}
	// 操作符前缀。
	op := ""
	for _, candidate := range []string{">=", "<=", "=", ">", "<", "^", "~"} {
		if strings.HasPrefix(s, candidate) {
			op = candidate
			s = strings.TrimSpace(s[len(candidate):])
			break
		}
	}
	// 取原子 token（到空格或逗号为止）。
	i := strings.IndexAny(s, " ,")
	verStr := s
	rest := ""
	if i >= 0 {
		verStr = strings.TrimSpace(s[:i])
		rest = s[i:]
	}
	v, err := semver.Parse(verStr)
	if err != nil {
		return Constraint{}, "", fmt.Errorf("invalid version in constraint: %w", err)
	}
	switch op {
	case "", "=":
		return Constraint{nodes: []Node{{op: "=", ver: v, lower: v, upper: v, inclL: true, inclU: true}}}, rest, nil
	case ">":
		return Constraint{nodes: []Node{{op: ">", ver: v}}}, rest, nil
	case "<":
		return Constraint{nodes: []Node{{op: "<", ver: v}}}, rest, nil
	case ">=":
		return Constraint{nodes: []Node{{op: ">=", ver: v}}}, rest, nil
	case "<=":
		return Constraint{nodes: []Node{{op: "<=", ver: v}}}, rest, nil
	case "^":
		return Constraint{nodes: []Node{{op: "^", lower: caretLower(v), upper: caretUpper(v), inclL: true}}}, rest, nil
	case "~":
		return Constraint{nodes: []Node{{op: "~", lower: tildeLower(v), upper: tildeUpper(v), inclL: true}}}, rest, nil
	}
	return Constraint{}, "", fmt.Errorf("unsupported operator %q", op)
}

func caretLower(v semver.Version) semver.Version {
	n := semver.Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch}
	n.Prerelease, n.Build = "", ""
	return n
}

func caretUpper(v semver.Version) semver.Version {
	if v.Major > 0 {
		return semver.Version{Major: v.Major + 1}
	}
	if v.Minor > 0 {
		return semver.Version{Major: 0, Minor: v.Minor + 1}
	}
	return semver.Version{Major: 0, Minor: 0, Patch: v.Patch + 1}
}

func tildeLower(v semver.Version) semver.Version {
	n := semver.Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch}
	return n
}

func tildeUpper(v semver.Version) semver.Version {
	return semver.Version{Major: v.Major, Minor: v.Minor + 1}
}

// Matches 判断版本是否满足约束。
func (c Constraint) Matches(v semver.Version) bool {
	for _, n := range c.nodes {
		if !n.match(v) {
			return false
		}
	}
	return true
}

func (n Node) match(v semver.Version) bool {
	switch n.op {
	case ">":
		return v.Compare(n.ver) > 0
	case "<":
		return v.Compare(n.ver) < 0
	case ">=":
		return v.Compare(n.ver) >= 0
	case "<=":
		return v.Compare(n.ver) <= 0
	case "=":
		return v.Compare(n.ver) == 0
	case "^", "~", "range":
		lc := v.Compare(n.lower)
		if lc < 0 || (lc == 0 && !n.inclL) {
			return false
		}
		uc := v.Compare(n.upper)
		if uc > 0 || (uc == 0 && !n.inclU) {
			return false
		}
		return true
	}
	return false
}

// String 返回原始约束。
func (c Constraint) String() string { return c.raw }
