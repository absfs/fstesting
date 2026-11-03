package fstesting

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// memSuite returns a Suite over a fresh memfs with every feature enabled.
func memSuite(t *testing.T) *Suite {
	t.Helper()
	return &Suite{FS: newMemFS(t), Features: DefaultFeatures()}
}

// skippedNames returns the paths, relative to root, of every skipped subtest.
func skippedNames(root *fakeT) []string {
	var out []string
	for _, c := range root.all() {
		if c.skipped {
			out = append(out, strings.TrimPrefix(c.name, root.name+"/"))
		}
	}
	return out
}

func TestMemSuitePassesWithoutSkips(t *testing.T) {
	s := memSuite(t)
	root := runFake(func(t tb) { s.run(t, nil) })
	root.assertNoPanic(t)
	for _, c := range root.all() {
		if c.failed {
			t.Errorf("%s failed: %s", c.name, c.errorText())
		}
	}
	if got := skippedNames(root); len(got) != 0 {
		t.Errorf("unexpected skips: %v", got)
	}
	if root.find("Symlinks/Lstat") == nil {
		t.Fatal("Symlinks/Lstat did not run")
	}
}

func TestSkipPatterns(t *testing.T) {
	tests := []struct {
		name    string
		skips   []Skip
		want    []string // skipped subtests, in run order
		running []string // subtests that must still run, not skipped
	}{
		{
			name:    "full path",
			skips:   []Skip{{Pattern: "Symlinks/Lstat", Reason: "route rewrites links"}},
			want:    []string{"Symlinks/Lstat"},
			running: []string{"Symlinks/CreateAndRead", "Symlinks/RelativeSymlink"},
		},
		{
			name:    "case insensitive",
			skips:   []Skip{{Pattern: "symlinks/LSTAT"}},
			want:    []string{"Symlinks/Lstat"},
			running: []string{"Symlinks/CreateAndRead"},
		},
		{
			name:    "bare name at any depth",
			skips:   []Skip{{Pattern: "CreateAndRead"}},
			want:    []string{"FileOperations/CreateAndRead", "Symlinks/CreateAndRead"},
			running: []string{"FileOperations/OpenFile", "Symlinks/Lstat"},
		},
		{
			name:    "path picks one of two same-named subtests",
			skips:   []Skip{{Pattern: "Symlinks/CreateAndRead"}},
			want:    []string{"Symlinks/CreateAndRead"},
			running: []string{"FileOperations/CreateAndRead"},
		},
		{
			name:    "whole group",
			skips:   []Skip{{Pattern: "Symlinks"}},
			want:    []string{"Symlinks"},
			running: []string{"FileOperations/CreateAndRead", "Permissions/Chmod"},
		},
		{
			name:  "glob within a group",
			skips: []Skip{{Pattern: "Symlinks/*Cycle"}, {Pattern: "ErrorSemantics/Not*"}},
			want: []string{
				"ErrorSemantics/NotExist", "ErrorSemantics/NotDir",
				"Symlinks/TwoNodeCycle",
			},
			running: []string{"Symlinks/CircularSymlinks", "ErrorSemantics/Exist"},
		},
		{
			name:    "star matches one path element",
			skips:   []Skip{{Pattern: "*/Lstat"}},
			want:    []string{"Symlinks/Lstat"},
			running: []string{"FileOperations/Stat"},
		},
		{
			name:    "names containing dots",
			skips:   []Skip{{Pattern: "NewFilerMethods/Filer.Sub"}},
			want:    []string{"NewFilerMethods/Filer.Sub"},
			running: []string{"NewFilerMethods/Filer.Sub.Nested"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := memSuite(t)
			s.Skips = tt.skips
			root := runFake(func(t tb) { s.run(t, s.Skips) })
			root.assertNoPanic(t)
			if root.failed {
				for _, c := range root.all() {
					if len(c.errors) > 0 {
						t.Errorf("%s: %s", c.name, c.errorText())
					}
				}
			}
			if got := skippedNames(root); strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("skipped %v, want %v", got, tt.want)
			}
			for _, name := range tt.running {
				if c := root.mustFind(t, name); c.skipped {
					t.Errorf("%s was skipped", name)
				}
			}
		})
	}
}

func TestSkipMessageStatesReason(t *testing.T) {
	s := memSuite(t)
	s.Skips = []Skip{
		{Pattern: "Symlinks/Lstat", Reason: "switchfs route rewrites link targets"},
		{Pattern: "Timestamps"},
	}
	root := runFake(func(t tb) { s.run(t, s.Skips) })

	got := root.mustFind(t, "Symlinks/Lstat").skipMsg
	for _, want := range []string{"Symlinks/Lstat", `"Symlinks/Lstat"`, "switchfs route rewrites link targets"} {
		if !strings.Contains(got, want) {
			t.Errorf("skip message %q lacks %q", got, want)
		}
	}
	if got := root.mustFind(t, "Timestamps").skipMsg; !strings.Contains(got, "no reason given") {
		t.Errorf("skip message without reason = %q", got)
	}
}

func TestInvalidSkipPatternFailsBeforeRunning(t *testing.T) {
	for _, p := range []string{"Symlinks/[", ""} {
		s := memSuite(t)
		root := runFake(func(t tb) { s.run(t, []Skip{{Pattern: p, Reason: "r"}}) })
		if !root.failed {
			t.Errorf("pattern %q: suite did not fail", p)
		}
		if len(root.children) != 0 {
			t.Errorf("pattern %q: %d subtests ran", p, len(root.children))
		}
		if !strings.Contains(root.errorText(), "skip pattern") {
			t.Errorf("pattern %q: error %q does not mention the skip pattern", p, root.errorText())
		}
	}
}

func TestUnmatchedSkipPatternIsLogged(t *testing.T) {
	s := memSuite(t)
	s.Features.Symlinks = false
	root := runFake(func(t tb) { s.run(t, []Skip{{Pattern: "Symlinks/Lstat"}, {Pattern: "Stat"}}) })
	logs := strings.Join(root.logs, "\n")
	if !strings.Contains(logs, `skip pattern "Symlinks/Lstat" matched no subtest`) {
		t.Errorf("missing unmatched-pattern log, got %q", logs)
	}
	if strings.Contains(logs, `"Stat"`) {
		t.Errorf("matched pattern logged as unmatched: %q", logs)
	}
}

// TestRunWithSkipsRealT runs both public entry points with a real *testing.T.
func TestRunWithSkipsRealT(t *testing.T) {
	s := memSuite(t)
	s.Skips = []Skip{{Pattern: "Timestamps", Reason: "exercising Suite.Skips"}}
	s.RunWithSkips(t, "Symlinks/Lstat", "PathHandling/Unicode")
	if len(s.Skips) != 1 {
		t.Errorf("RunWithSkips modified s.Skips: %v", s.Skips)
	}
	s.Run(t)
}

// TestRunWithSkipsGoTestOutput checks, through a real `go test -v` run of
// this binary, that subtests selected by RunWithSkips patterns and by
// Suite.Skips are reported as SKIP with their reasons and that their siblings
// still run.
func TestRunWithSkipsGoTestOutput(t *testing.T) {
	out := runHelper(t, "skips")
	for _, want := range []string{
		"--- SKIP: TestHelperProcess/Symlinks/Lstat",
		`fstesting: skipping Symlinks/Lstat (skip pattern "Symlinks/Lstat"): ` + runWithSkipsReason,
		"--- PASS: TestHelperProcess/Symlinks/CreateAndRead",
		"--- PASS: TestHelperProcess/FileOperations/CreateAndRead",
		"--- SKIP: TestHelperProcess/Timestamps",
		`fstesting: skipping Timestamps (skip pattern "Timestamps"): no clock`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

// runHelper reruns this test binary to execute TestHelperProcess in the given
// mode and returns its combined output.
func runHelper(t *testing.T, mode string) string {
	t.Helper()
	args := []string{"-test.run=^TestHelperProcess$", "-test.v"}
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "FSTESTING_HELPER="+mode)
	out, _ := cmd.CombinedOutput() // the "rewrite" mode is expected to fail
	return string(out)
}

// TestHelperProcess is not a real test; runHelper invokes it in a child
// process so that its output, including failures, can be inspected.
func TestHelperProcess(t *testing.T) {
	switch os.Getenv("FSTESTING_HELPER") {
	case "skips":
		s := memSuite(t)
		s.Skips = []Skip{{Pattern: "Timestamps", Reason: "no clock"}}
		s.RunWithSkips(t, "Symlinks/Lstat")
	case "rewrite":
		s := &Suite{FS: newRewriteFS(t), Features: DefaultFeatures()}
		s.Run(t)
	default:
		t.Skip("helper process only")
	}
}

// TestRunWithSkipsSkipsPatterns drives the RunWithSkips path in process, so a
// regression to ignoring the patterns fails here and not only in the child
// process test.
func TestRunWithSkipsSkipsPatterns(t *testing.T) {
	s := memSuite(t)
	s.Skips = []Skip{{Pattern: "Timestamps", Reason: "no clock"}}
	root := runFake(func(t tb) { s.runWithSkips(t, []string{"Symlinks/Lstat", "PathHandling/Unicode"}) })
	root.assertNoPanic(t)

	got := strings.Join(skippedNames(root), ",")
	if want := "PathHandling/Unicode,Symlinks/Lstat,Timestamps"; got != want {
		t.Errorf("skipped %s, want %s", got, want)
	}
	if msg := root.mustFind(t, "Symlinks/Lstat").skipMsg; !strings.Contains(msg, runWithSkipsReason) {
		t.Errorf("skip message %q lacks %q", msg, runWithSkipsReason)
	}
	if c := root.mustFind(t, "Symlinks/CreateAndRead"); c.skipped || c.failed {
		t.Errorf("sibling Symlinks/CreateAndRead skipped=%v failed=%v", c.skipped, c.failed)
	}
	if len(s.Skips) != 1 {
		t.Errorf("runWithSkips modified s.Skips: %v", s.Skips)
	}
}

// TestShadowedSkipPatternsAreNotLogged checks that a pattern whose subtest
// was skipped by an earlier rule, either a duplicate or an enclosing group, is
// not reported as matching nothing.
func TestShadowedSkipPatternsAreNotLogged(t *testing.T) {
	s := memSuite(t)
	skips := []Skip{
		{Pattern: "FileOperations/CreateAndRead", Reason: "r"},
		{Pattern: "fileoperations/createandread"},
		{Pattern: "Symlinks"},
		{Pattern: "Symlinks/Lstat"},
		{Pattern: "Symlinks/*Symlink"},
		{Pattern: "Symlinks/Nope/Deeper"},
		{Pattern: "Timestamps/NoSuchTest"},
	}
	root := runFake(func(t tb) { s.run(t, skips) })

	if got := strings.Join(skippedNames(root), ","); got != "FileOperations/CreateAndRead,Symlinks" {
		t.Errorf("skipped %s", got)
	}
	logs := strings.Join(root.logs, "\n")
	if strings.Count(logs, "matched no subtest") != 1 || !strings.Contains(logs, `"Timestamps/NoSuchTest"`) {
		t.Errorf("want only Timestamps/NoSuchTest logged as unmatched, got %q", logs)
	}
}

// TestNestedSuiteSkipPatterns covers a suite run inside a subtest: patterns
// are relative to that subtest, and a pattern that repeats its name is
// reported with a hint to drop the prefix.
func TestNestedSuiteSkipPatterns(t *testing.T) {
	s := memSuite(t)
	root := &fakeT{name: "TestMyFS/memfs"}
	root.exec(func(t tb) { s.runWithSkips(t, []string{"Symlinks/Lstat", "memfs/Symlinks/RelativeSymlink"}) })

	if !root.mustFind(t, "Symlinks/Lstat").skipped {
		t.Error("Symlinks/Lstat was not skipped")
	}
	if root.mustFind(t, "Symlinks/RelativeSymlink").skipped {
		t.Error("Symlinks/RelativeSymlink was skipped by a pattern naming the parent test")
	}
	want := `fstesting: skip pattern "memfs/Symlinks/RelativeSymlink" matched no subtest of TestMyFS/memfs (patterns are relative to that test); drop the leading "memfs/"`
	if logs := strings.Join(root.logs, "\n"); logs != want {
		t.Errorf("logs = %q, want %q", logs, want)
	}
}

func TestParentPrefix(t *testing.T) {
	tests := []struct {
		root, pattern, want string
		ok                  bool
	}{
		{"TestMyFS/memfs", "memfs/Symlinks/Lstat", "memfs/", true},
		{"TestMyFS/memfs", "TestMyFS/memfs/Symlinks/Lstat", "TestMyFS/memfs/", true},
		{"TestMyFS/MemFS", "memfs/Symlinks/Lstat", "memfs/", true},
		{"TestMyFS", "Symlinks/Lstat", "", false},
		{"TestMyFS/memfs", "memfs", "", false},
		{"TestMyFS/memfs", "Lstat", "", false},
	}
	for _, tt := range tests {
		got, ok := parentPrefix(tt.root, tt.pattern)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parentPrefix(%q, %q) = %q, %v; want %q, %v", tt.root, tt.pattern, got, ok, tt.want, tt.ok)
		}
	}
}

// TestSetupFailureDoesNotLogUnmatched checks that when the test directory
// cannot be created, the skip patterns are not misreported as typos.
func TestSetupFailureDoesNotLogUnmatched(t *testing.T) {
	f := newFaultFS(t)
	f.fail = failOps("MkdirAll")
	s := &Suite{FS: f, Features: DefaultFeatures()}
	root := runFake(func(t tb) { s.run(t, []Skip{{Pattern: "Symlinks/Lstat"}}) })
	if !root.failed {
		t.Fatal("suite did not fail")
	}
	if logs := strings.Join(root.logs, "\n"); strings.Contains(logs, "matched no subtest") {
		t.Errorf("setup failure logged unmatched patterns: %q", logs)
	}
}
