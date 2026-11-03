package fstesting

import (
	"os"
	"strings"
	"testing"

	"github.com/absfs/absfs"
	"github.com/absfs/memfs"
)

// The Fuzz* targets below run each helper's seed corpus during `go test`
// and can be fuzzed with, for example, `go test -fuzz=FuzzMemReadWrite`.

func memFSForFuzz(f *testing.F) absfs.FileSystem {
	m, err := memfs.NewFS()
	if err != nil {
		f.Fatal(err)
	}
	return m
}

func FuzzMemCreate(f *testing.F)        { FuzzCreate(f, memFSForFuzz(f), "/fuzz") }
func FuzzMemReadWrite(f *testing.F)     { FuzzReadWrite(f, memFSForFuzz(f), "/fuzz") }
func FuzzMemRename(f *testing.F)        { FuzzRename(f, memFSForFuzz(f), "/fuzz") }
func FuzzMemMkdir(f *testing.F)         { FuzzMkdir(f, memFSForFuzz(f), "/fuzz") }
func FuzzMemPathTraversal(f *testing.F) { FuzzPathTraversal(f, memFSForFuzz(f), "/fuzz") }
func FuzzMemOpenFlags(f *testing.F)     { FuzzOpenFlags(f, memFSForFuzz(f), "/fuzz") }

func FuzzMemWrapperRoundtrip(f *testing.F) {
	FuzzWrapperRoundtrip(f, identity, memFSForFuzz(f), "/fuzz")
}

// fuzzCase describes one call of a fuzz iteration against a misbehaving
// filesystem and the failure it must report ("" means it must pass).
type fuzzCase struct {
	name string
	fs   func(*testing.T) absfs.FileSystem
	run  func(t tb, fs absfs.FileSystem)
	want string
}

func faulty(configure func(*faultFS)) func(*testing.T) absfs.FileSystem {
	return func(t *testing.T) absfs.FileSystem {
		f := newFaultFS(t)
		configure(f)
		return f
	}
}

var (
	healthy = faulty(func(*faultFS) {})
	liar    = faulty(func(f *faultFS) { f.lie = true })
	corrupt = faulty(func(f *faultFS) { f.corrupt = true })
	failing = func(ops ...string) func(*testing.T) absfs.FileSystem {
		return faulty(func(f *faultFS) { f.fail = failOps(ops...) })
	}
	badFiles = func(mode string) func(*testing.T) absfs.FileSystem {
		return func(t *testing.T) absfs.FileSystem { return &badFileFS{faultFS: newFaultFS(t), mode: mode} }
	}
)

func runFuzzCases(t *testing.T, cases []fuzzCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fsys := tc.fs(t)
			root := runFake(func(t tb) { tc.run(t, fsys) })
			root.assertNoPanic(t)
			if tc.want == "" {
				if root.failed {
					t.Errorf("failed: %s", root.errorText())
				}
				return
			}
			if !strings.Contains(root.errorText(), tc.want) {
				t.Errorf("errors %q lack %q", root.errorText(), tc.want)
			}
		})
	}
}

func TestFuzzCreateCase(t *testing.T) {
	c := func(name string) func(tb, absfs.FileSystem) {
		return func(t tb, fs absfs.FileSystem) { fuzzCreateCase(t, fs, "/fuzz", name) }
	}
	runFuzzCases(t, []fuzzCase{
		{"empty", healthy, c(""), ""},
		{"nul", healthy, c("a\x00b"), ""},
		{"invalid utf8", healthy, c("\xff"), ""},
		{"mkdir fails", failing("MkdirAll"), c("a/b"), ""},
		{"create fails", failing("Create"), c("a"), ""},
		{"stat fails", failing("Stat"), c("a"), "Stat failed"},
		{"reported as dir", liar, c("a"), "IsDir() returned true"},
	})
}

func TestFuzzReadWriteCase(t *testing.T) {
	c := func(counter int, data string) func(tb, absfs.FileSystem) {
		return func(t tb, fs absfs.FileSystem) { fuzzReadWriteCase(t, fs, "/fuzz", counter, []byte(data)) }
	}
	runFuzzCases(t, []fuzzCase{
		{"ok", healthy, c(1, "data"), ""},
		{"create fails", failing("Create"), c(1, "data"), "Create failed"},
		{"write fails", badFiles("write"), c(1, "data"), ""},
		{"open fails", failing("Open"), c(1, "data"), "Open failed"},
		{"read fails", badFiles("read"), c(1, "data"), "ReadAll failed"},
		{"short write", liar, c(1, "data"), "Write returned"},
		{"corrupt read", corrupt, c(1, "data"), "data mismatch"},
	})
}

func TestFuzzRenameCase(t *testing.T) {
	c := func(oldName, newName string) func(tb, absfs.FileSystem) {
		return func(t tb, fs absfs.FileSystem) { fuzzRenameCase(t, fs, "/fuzz", oldName, newName) }
	}
	runFuzzCases(t, []fuzzCase{
		{"ok", healthy, c("a", "sub/b"), ""},
		{"empty", healthy, c("", "b"), ""},
		{"nul", healthy, c("a\x00", "b"), ""},
		{"dot", healthy, c("a", ".."), ""},
		{"create fails", failing("Create"), c("a", "b"), ""},
		{"rename fails", failing("Rename"), c("a", "b"), ""},
		{"rename lies", liar, c("a", "b"), "old path should not exist"},
		{"open fails", failing("Open"), c("a", "b"), "cannot open renamed file"},
		{"corrupt read", corrupt, c("a", "b"), "content mismatch after rename"},
	})
}

func TestFuzzMkdirCase(t *testing.T) {
	c := func(name string) func(tb, absfs.FileSystem) {
		return func(t tb, fs absfs.FileSystem) { fuzzMkdirCase(t, fs, "/fuzz", name) }
	}
	runFuzzCases(t, []fuzzCase{
		{"ok", healthy, c("a/b"), ""},
		{"empty", healthy, c(""), ""},
		{"dot", healthy, c("."), ""},
		{"mkdir fails", failing("MkdirAll"), c("a"), ""},
		{"stat fails", failing("Stat"), c("a"), "Stat failed"},
		{"not a dir", liar, c("a"), "not a directory"},
	})
}

func TestFuzzPathTraversalCase(t *testing.T) {
	c := func(p string) func(tb, absfs.FileSystem) {
		return func(t tb, fs absfs.FileSystem) { fuzzPathTraversalCase(t, fs, "/fuzz", p) }
	}
	runFuzzCases(t, []fuzzCase{
		{"empty", healthy, c(""), ""},
		{"escape", healthy, c("../../escape"), ""},
	})
}

func TestFuzzOpenFlagsCase(t *testing.T) {
	c := func(counter, flags int) func(tb, absfs.FileSystem) {
		return func(t tb, fs absfs.FileSystem) { fuzzOpenFlagsCase(t, fs, "/fuzz", counter, flags) }
	}
	runFuzzCases(t, []fuzzCase{
		{"read existing", healthy, c(2, os.O_RDONLY), ""},
		{"write new", healthy, c(1, os.O_CREATE|os.O_WRONLY), ""},
		{"rdwr existing", healthy, c(2, os.O_RDWR), ""},
		{"missing", healthy, c(1, os.O_RDONLY), ""},
		{"open fails", failing("OpenFile"), c(2, os.O_RDONLY), ""},
	})
}

func TestFuzzWrapperRoundtripCase(t *testing.T) {
	c := func(data string) func(tb, absfs.FileSystem) {
		return func(t tb, fs absfs.FileSystem) { fuzzWrapperRoundtripCase(t, fs, "/fuzz", 3, []byte(data)) }
	}
	runFuzzCases(t, []fuzzCase{
		{"ok", healthy, c("data"), ""},
		{"create fails", failing("Create"), c("data"), ""},
		{"open fails", failing("Open"), c("data"), "Open failed after successful Create"},
		{"read fails", badFiles("read"), c("data"), "ReadAll failed"},
		{"corrupt read", corrupt, c("data"), "roundtrip failed"},
	})
}

func TestNodeTypeString(t *testing.T) {
	want := []string{"Unknown", "Core", "Implementation", "Adapter", "Wrapper", "Compositor", "Consumer"}
	for i, w := range want {
		if got := NodeType(i).String(); got != w {
			t.Errorf("NodeType(%d) = %q, want %q", i, got, w)
		}
	}
}

func TestMinimalFeatures(t *testing.T) {
	if (MinimalFeatures() != Features{CaseSensitive: true}) {
		t.Errorf("MinimalFeatures() = %+v", MinimalFeatures())
	}
}
