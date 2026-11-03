package fstesting

import (
	"fmt"
	"path"
	"strings"
	"sync/atomic"
	"testing"
)

// Skip selects suite subtests to skip and records why.
//
// Pattern is matched case-insensitively against subtest names exactly as the
// suite spells them, which is also how `go test -v` prints them, minus the name
// of the test function that called Run. Matching uses [path.Match] syntax
// ('*', '?', '[...]'), and '*' never crosses a '/':
//
//   - A pattern containing '/' is matched against the subtest's full path below
//     the suite root, e.g. "Symlinks/Lstat" or "Symlinks/*Symlink".
//   - A pattern without '/' is matched against the subtest's own name at any
//     depth, e.g. "Lstat" skips Symlinks/Lstat and "Symlinks" skips the whole
//     Symlinks group. A bare name can match several subtests (both
//     FileOperations/CreateAndRead and Symlinks/CreateAndRead are named
//     "CreateAndRead"); use the path form to pick exactly one.
//
// Skipping a group skips everything under it. Each skipped subtest calls
// t.Skip with a message that names the subtest, the pattern that matched, and
// Reason, so the skip is visible in `go test -v` output.
type Skip struct {
	// Pattern selects the subtests to skip. Required.
	Pattern string

	// Reason explains the skip, typically a known limitation of the
	// filesystem under test. It is included in the t.Skip message.
	Reason string
}

// runWithSkipsReason is the reason reported for patterns passed to
// RunWithSkips, which has no way to carry one.
const runWithSkipsReason = "listed in Suite.RunWithSkips"

// tb is the subset of *testing.T that the suites use. It lets this package's
// own tests drive the suites with a recording fake, so failure paths can be
// exercised without failing the enclosing test.
type tb interface {
	Helper()
	Name() string
	Error(args ...any)
	Errorf(format string, args ...any)
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Skip(args ...any)
	Logf(format string, args ...any)
	Cleanup(f func())
	Run(name string, f func(t tb)) bool
}

// realT adapts *testing.T to tb.
type realT struct{ *testing.T }

func (r realT) Run(name string, f func(t tb)) bool {
	return r.T.Run(name, func(t *testing.T) { f(realT{t}) })
}

// skipRule is a compiled Skip.
type skipRule struct {
	Skip
	lower   string // lower-cased pattern
	full    bool   // pattern contains '/': match the full relative path
	matched atomic.Int64
}

func (r *skipRule) matches(rel, name string) bool {
	target := name
	if r.full {
		target = rel
	}
	ok, _ := path.Match(r.lower, strings.ToLower(target)) // pattern validated by compileSkips
	return ok
}

// compileSkips validates skips and compiles them into rules.
func compileSkips(skips []Skip) ([]*skipRule, error) {
	rules := make([]*skipRule, 0, len(skips))
	for _, s := range skips {
		if s.Pattern == "" {
			return nil, fmt.Errorf("fstesting: empty skip pattern (reason %q)", s.Reason)
		}
		lower := strings.ToLower(s.Pattern)
		if _, err := path.Match(lower, ""); err != nil {
			return nil, fmt.Errorf("fstesting: invalid skip pattern %q: %v", s.Pattern, err)
		}
		rules = append(rules, &skipRule{
			Skip:  s,
			lower: lower,
			full:  strings.Contains(s.Pattern, "/"),
		})
	}
	return rules, nil
}

// skipT wraps a tb so that every subtest started through it is checked
// against the skip rules before its body runs.
type skipT struct {
	tb
	rules []*skipRule
	rel   string // path of this test below the suite root; "" at the root
}

func (s skipT) Run(name string, f func(t tb)) bool {
	rel := name
	if s.rel != "" {
		rel = s.rel + "/" + name
	}
	return s.tb.Run(name, func(t tb) {
		child := skipT{tb: t, rules: s.rules, rel: rel}
		for _, r := range s.rules {
			if !r.matches(rel, name) {
				continue
			}
			r.matched.Add(1)
			reason := r.Reason
			if reason == "" {
				reason = "no reason given"
			}
			t.Skip(fmt.Sprintf("fstesting: skipping %s (skip pattern %q): %s", rel, r.Pattern, reason))
			return
		}
		f(child)
	})
}

// logUnmatched reports skip patterns that selected nothing, which usually
// means a typo or a renamed subtest. It is a log line, not a failure: a
// pattern may legitimately match nothing when its group is disabled by
// Features or filtered out by -run.
func logUnmatched(t tb, rules []*skipRule) {
	for _, r := range rules {
		if r.matched.Load() == 0 {
			t.Logf("fstesting: skip pattern %q matched no subtest", r.Pattern)
		}
	}
}
