package fstesting

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// Skip selects suite subtests to skip and records why.
//
// Pattern is matched case-insensitively against subtest names exactly as the
// suite spells them. Names are relative to the *testing.T passed to Run or
// RunWithSkips: in `go test -v` output they are the part after that test's
// full name. When Run is called directly from TestMyFS, Symlinks/Lstat prints
// as TestMyFS/Symlinks/Lstat and the pattern is "Symlinks/Lstat". When Run is
// called inside a subtest, for example t.Run("memfs", ...), the test prints as
// TestMyFS/memfs/Symlinks/Lstat and the pattern is still "Symlinks/Lstat", not
// "memfs/Symlinks/Lstat". Matching uses [path.Match] syntax ('*', '?',
// '[...]'), and '*' never crosses a '/':
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
		var first *skipRule
		for _, r := range s.rules {
			if !r.matches(rel, name) {
				continue
			}
			r.matched.Add(1)
			if first == nil {
				first = r
			}
		}
		if first == nil {
			f(skipT{tb: t, rules: s.rules, rel: rel})
			return
		}
		// Skipping rel also skips everything below it, so path patterns that
		// select a descendant are satisfied too; credit them so they are not
		// reported as unmatched.
		for _, r := range s.rules {
			if r.below(rel) {
				r.matched.Add(1)
			}
		}
		reason := first.Reason
		if reason == "" {
			reason = "no reason given"
		}
		t.Skip(fmt.Sprintf("fstesting: skipping %s (skip pattern %q): %s", rel, first.Pattern, reason))
	})
}

// below reports whether r is a path pattern whose leading elements match rel,
// so that it selects only subtests below rel.
func (r *skipRule) below(rel string) bool {
	if !r.full {
		return false
	}
	pat := strings.Split(r.lower, "/")
	elems := strings.Split(strings.ToLower(rel), "/")
	if len(pat) <= len(elems) {
		return false
	}
	for i, e := range elems {
		if ok, _ := path.Match(pat[i], e); !ok {
			return false
		}
	}
	return true
}

// logUnmatched reports skip patterns that selected nothing, which usually
// means a typo, a renamed subtest, or a pattern written relative to the wrong
// test. It is a log line, not a failure: a pattern may legitimately match
// nothing when its group is disabled by Features or filtered out by -run.
func logUnmatched(t tb, rules []*skipRule) {
	t.Helper()
	for _, r := range rules {
		if r.matched.Load() != 0 {
			continue
		}
		msg := fmt.Sprintf("fstesting: skip pattern %q matched no subtest of %s (patterns are relative to that test)", r.Pattern, t.Name())
		if prefix, ok := parentPrefix(t.Name(), r.Pattern); ok {
			msg += fmt.Sprintf("; drop the leading %q", prefix)
		}
		t.Logf("%s", msg)
	}
}

// parentPrefix reports whether pattern begins with trailing elements of the
// root test name, the mistake of copying a subtest name from `go test -v`
// output when the suite runs inside a subtest. It returns the prefix to drop.
func parentPrefix(rootName, pattern string) (string, bool) {
	names := strings.Split(strings.ToLower(rootName), "/")
	pat := strings.Split(strings.ToLower(pattern), "/")
	for i := range names {
		tail := names[i:]
		if len(tail) >= len(pat) || !slices.Equal(tail, pat[:len(tail)]) {
			continue
		}
		return strings.Join(strings.Split(pattern, "/")[:len(tail)], "/") + "/", true
	}
	return "", false
}
