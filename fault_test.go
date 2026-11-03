package fstesting

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/absfs/absfs"
	"github.com/absfs/memfs"
)

var errFault = errors.New("injected fault")

// faultFS wraps memfs to model filesystems that misbehave in the ways the
// suites must survive: failing operations, lying results, and a routing layer
// that rewrites symlink targets.
type faultFS struct {
	*memfs.FileSystem

	// fail reports whether op on name returns an injected error.
	fail func(op, name string) bool

	// lie makes operations report success and return wrong data.
	lie bool

	// rewrite stores every symlink target under /elsewhere, the way a
	// path-rewriting route can, so links no longer resolve to their targets.
	rewrite bool

	// bareSub makes Sub return an fs.FS with no optional interfaces.
	bareSub bool

	// corrupt makes files returned by Open corrupt what they read, without
	// any of the other lies.
	corrupt bool
}

var _ absfs.SymlinkFileSystem = (*faultFS)(nil)

func newFaultFS(t *testing.T) *faultFS { return &faultFS{FileSystem: newMemFS(t)} }

// newRewriteFS models the switchfs route that rewrote symlink targets.
func newRewriteFS(t *testing.T) *faultFS {
	f := newFaultFS(t)
	f.rewrite = true
	return f
}

// failAll fails every operation except creating the suite's own test
// directory, so each subtest hits its first error.
func failAll(op, name string) bool {
	return !(op == "MkdirAll" && strings.HasPrefix(path.Base(name), "fstesting_"))
}

// failOps returns a fail func that fails exactly the named operations.
func failOps(ops ...string) func(op, name string) bool {
	return func(op, _ string) bool {
		for _, o := range ops {
			if o == op {
				return true
			}
		}
		return false
	}
}

func (f *faultFS) err(op, name string) error {
	if f.fail != nil && f.fail(op, name) {
		return &os.PathError{Op: op, Path: name, Err: errFault}
	}
	return nil
}

func (f *faultFS) Open(name string) (absfs.File, error) {
	if err := f.err("Open", name); err != nil {
		return nil, err
	}
	file, err := f.FileSystem.Open(name)
	if f.corrupt && err == nil {
		return &lieFile{File: file}, nil
	}
	if !f.lie {
		return file, err
	}
	if err != nil {
		return nil, errors.New("lie: not a not-exist error")
	}
	return &lieFile{File: file}, nil
}

func (f *faultFS) Create(name string) (absfs.File, error) {
	if err := f.err("Create", name); err != nil {
		return nil, err
	}
	file, err := f.FileSystem.Create(name)
	if f.lie && err == nil {
		return &lieFile{File: file}, nil
	}
	return file, err
}

func (f *faultFS) OpenFile(name string, flag int, perm os.FileMode) (absfs.File, error) {
	if err := f.err("OpenFile", name); err != nil {
		return nil, err
	}
	file, err := f.FileSystem.OpenFile(name, flag, perm)
	if !f.lie || err == nil {
		return file, err
	}
	// Claim success for opens that should have failed (O_EXCL on an
	// existing file, a directory opened for writing).
	file, err = f.FileSystem.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return &lieFile{File: file}, nil
}

func (f *faultFS) Mkdir(name string, perm os.FileMode) error {
	if err := f.err("Mkdir", name); err != nil {
		return err
	}
	err := f.FileSystem.Mkdir(name, perm)
	if f.lie {
		return nil
	}
	return err
}

func (f *faultFS) MkdirAll(name string, perm os.FileMode) error {
	if err := f.err("MkdirAll", name); err != nil {
		return err
	}
	return f.FileSystem.MkdirAll(name, perm)
}

func (f *faultFS) Remove(name string) error {
	if err := f.err("Remove", name); err != nil || f.lie {
		return err
	}
	return f.FileSystem.Remove(name)
}

func (f *faultFS) RemoveAll(name string) error {
	if err := f.err("RemoveAll", name); err != nil || f.lie {
		return err
	}
	return f.FileSystem.RemoveAll(name)
}

func (f *faultFS) Rename(oldpath, newpath string) error {
	if err := f.err("Rename", oldpath); err != nil || f.lie {
		return err
	}
	return f.FileSystem.Rename(oldpath, newpath)
}

func (f *faultFS) Truncate(name string, size int64) error {
	if err := f.err("Truncate", name); err != nil || f.lie {
		return err
	}
	return f.FileSystem.Truncate(name, size)
}

func (f *faultFS) Chmod(name string, mode os.FileMode) error {
	if err := f.err("Chmod", name); err != nil || f.lie {
		return err
	}
	return f.FileSystem.Chmod(name, mode)
}

func (f *faultFS) Chtimes(name string, atime, mtime time.Time) error {
	if err := f.err("Chtimes", name); err != nil || f.lie {
		return err
	}
	return f.FileSystem.Chtimes(name, atime, mtime)
}

func (f *faultFS) Stat(name string) (os.FileInfo, error) {
	if err := f.err("Stat", name); err != nil {
		return nil, err
	}
	info, err := f.FileSystem.Stat(name)
	if f.lie {
		return lieInfo{info}, nil
	}
	return info, err
}

func (f *faultFS) Lstat(name string) (os.FileInfo, error) {
	if err := f.err("Lstat", name); err != nil {
		return nil, err
	}
	info, err := f.FileSystem.Lstat(name)
	if f.lie {
		return lieInfo{info}, nil
	}
	return info, err
}

func (f *faultFS) Readlink(name string) (string, error) {
	if err := f.err("Readlink", name); err != nil {
		return "", err
	}
	if f.lie {
		return "lie", nil
	}
	return f.FileSystem.Readlink(name)
}

func (f *faultFS) Symlink(oldname, newname string) error {
	if err := f.err("Symlink", newname); err != nil {
		return err
	}
	if f.rewrite {
		oldname = path.Join("/elsewhere", oldname)
	}
	err := f.FileSystem.Symlink(oldname, newname)
	if f.lie {
		return nil
	}
	return err
}

func (f *faultFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := f.err("ReadDir", name); err != nil {
		return nil, err
	}
	if f.lie {
		return []fs.DirEntry{lieEntry{}}, nil
	}
	return f.FileSystem.ReadDir(name)
}

func (f *faultFS) ReadFile(name string) ([]byte, error) {
	if err := f.err("ReadFile", name); err != nil {
		return nil, err
	}
	data, err := f.FileSystem.ReadFile(name)
	if f.lie {
		return append(data, '!'), nil
	}
	return data, err
}

func (f *faultFS) Sub(dir string) (fs.FS, error) {
	if err := f.err("Sub", dir); err != nil {
		return nil, err
	}
	if f.bareSub {
		return struct{ fs.FS }{fstest.MapFS{}}, nil
	}
	if f.lie {
		return fstest.MapFS{"nested/file.txt": {Data: []byte("lie")}}, nil
	}
	return f.FileSystem.Sub(dir)
}

// lieFile reports short writes, corrupts reads, and pretends files list as
// directories.
type lieFile struct{ absfs.File }

func (l *lieFile) Write(b []byte) (int, error) {
	n, err := l.File.Write(b)
	if n > 0 {
		n--
	}
	return n, err
}

func (l *lieFile) Read(b []byte) (int, error) {
	n, err := l.File.Read(b)
	if n > 0 {
		b[0] ^= 0xff
	}
	return n, err
}

func (l *lieFile) Readdir(int) ([]os.FileInfo, error) { return nil, nil }

func (l *lieFile) ReadDir(int) ([]fs.DirEntry, error) { return []fs.DirEntry{lieEntry{}}, nil }

// lieInfo contradicts every field of the FileInfo it wraps, which may be nil.
type lieInfo struct{ os.FileInfo }

func (l lieInfo) Name() string { return "lie" }

func (l lieInfo) Size() int64 {
	if l.FileInfo == nil {
		return -1
	}
	return l.FileInfo.Size() + 1
}

func (l lieInfo) IsDir() bool { return l.FileInfo == nil || !l.FileInfo.IsDir() }

func (l lieInfo) Mode() os.FileMode {
	if l.FileInfo == nil {
		return os.ModeSymlink | 0777
	}
	return (l.FileInfo.Mode() ^ os.ModeSymlink) | 0777
}

func (l lieInfo) ModTime() time.Time { return time.Time{} }
func (l lieInfo) Sys() any           { return nil }

// lieEntry is a directory entry with no name whose Info fails.
type lieEntry struct{}

func (lieEntry) Name() string               { return "" }
func (lieEntry) IsDir() bool                { return false }
func (lieEntry) Type() fs.FileMode          { return 0 }
func (lieEntry) Info() (fs.FileInfo, error) { return nil, errFault }

// runSuite runs the full suite over fsys against a fakeT and checks that no
// subtest panicked.
func runSuite(t *testing.T, fsys absfs.FileSystem) *fakeT {
	t.Helper()
	s := &Suite{FS: fsys, Features: DefaultFeatures()}
	root := runFake(func(t tb) { s.run(t, nil) })
	root.assertNoPanic(t)
	return root
}

// assertFailed checks that each named subtest ran and failed with a message
// containing the paired substring.
func assertFailed(t *testing.T, root *fakeT, want map[string]string) {
	t.Helper()
	for name, substr := range want {
		c := root.mustFind(t, name)
		if !c.failed {
			t.Errorf("%s passed, want failure", name)
			continue
		}
		if !strings.Contains(c.errorText(), substr) {
			t.Errorf("%s errors %q lack %q", name, c.errorText(), substr)
		}
	}
}

// TestLstatFailsCleanlyWhenSymlinkDoesNotResolve is the switchfs regression:
// over a route that rewrites symlink targets, Stat on the link fails, and the
// Lstat subtest used to dereference the nil FileInfo it returned.
func TestLstatFailsCleanlyWhenSymlinkDoesNotResolve(t *testing.T) {
	root := runSuite(t, newRewriteFS(t))
	assertFailed(t, root, map[string]string{
		"Symlinks/Lstat":         "lstat_link failed",
		"Symlinks/CreateAndRead": "Readlink",
	})
	if c := root.mustFind(t, "FileOperations"); c.failed {
		t.Errorf("FileOperations failed: unrelated to symlinks")
	}
}

// TestLstatFailsCleanlyGoTestOutput runs the same regression in a child
// `go test` process with a real *testing.T and checks the report.
func TestLstatFailsCleanlyGoTestOutput(t *testing.T) {
	out := runHelper(t, "rewrite")
	if strings.Contains(out, "panic:") {
		t.Fatalf("suite panicked:\n%s", out)
	}
	for _, want := range []string{"--- FAIL: TestHelperProcess/Symlinks/Lstat", "lstat_link failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestSuiteFailsCleanlyWhenEverythingFails(t *testing.T) {
	f := newFaultFS(t)
	f.fail = failAll
	root := runSuite(t, f)
	for _, c := range root.all() {
		// ErrorSemantics/IsDir passes: it only checks that an operation fails.
		if len(c.children) == 0 && !c.failed && !strings.HasSuffix(c.name, "/ErrorSemantics/IsDir") {
			t.Errorf("%s passed although every operation failed", c.name)
		}
	}
}

func TestSuiteFailsCleanlyWhenStatFails(t *testing.T) {
	f := newFaultFS(t)
	f.fail = failOps("Stat", "Lstat", "Readlink", "Rename", "Truncate", "Chmod", "Chtimes", "ReadFile")
	f.bareSub = true
	root := runSuite(t, f)
	assertFailed(t, root, map[string]string{
		"FileOperations/Stat":        "Stat",
		"PathHandling/TrailingSlash": "trailing slash",
		"PathHandling/Unicode":       "Stat unicode",
		"ErrorSemantics/NotExist":    "expected os.IsNotExist",
		"NewFilerMethods/Filer.Sub":  "fs.ReadFileFS",
		"Symlinks/Lstat":             "Stat",
		"Symlinks/RemoveSymlink":     "Target file should still exist",
		"Symlinks/RelativeSymlink":   "Readlink failed",
	})
}

func TestSuiteFailsCleanlyWhenLiedTo(t *testing.T) {
	f := newFaultFS(t)
	f.lie = true
	root := runSuite(t, f)
	assertFailed(t, root, map[string]string{
		"FileOperations/CreateAndRead":    "Write returned",
		"FileOperations/OpenFile":         "O_EXCL",
		"FileOperations/Truncate":         "size after truncate",
		"FileOperations/Remove":           "should not exist",
		"FileOperations/Rename":           "old path should not exist",
		"FileOperations/Stat":             "Name",
		"DirectoryOperations/Mkdir":       "directory",
		"DirectoryOperations/RemoveAll":   "should not exist",
		"DirectoryOperations/ReadDir":     "empty name",
		"ErrorSemantics/NotExist":         "expected os.IsNotExist",
		"ErrorSemantics/Exist":            "expected os.IsExist",
		"ErrorSemantics/IsDir":            "should fail",
		"ErrorSemantics/NotDir":           "should fail",
		"NewFilerMethods/Filer.ReadDir":   "Info() failed",
		"NewFilerMethods/Filer.ReadFile":  "content mismatch",
		"NewFilerMethods/Filer.Sub":       "content mismatch",
		"Symlinks/CreateAndRead":          "Readlink",
		"Symlinks/Lstat":                  "Stat should follow symlink",
		"Symlinks/SymlinkAlreadyExists":   "should fail",
		"Symlinks/BrokenSymlink":          "Readlink",
		"Symlinks/CircularSymlinks":       "symlink mode",
		"Symlinks/SelfReferenceSymlink":   "symlink mode",
		"Symlinks/TwoNodeCycle":           "symlink mode",
		"Symlinks/RemoveSymlink":          "should be removed",
		"Symlinks/ReadThroughSymlink":     "content mismatch",
		"Symlinks/WriteThroughSymlink":    "did not go to target",
		"Symlinks/ChainedSymlinks":        "Readlink link2",
		"Symlinks/RenameSymlink":          "should not exist",
		"Symlinks/SameDirRelativeSymlink": "Readlink",
		"Symlinks/SymlinkToDirectory":     "directory",
		"Permissions/Chmod":               "mode after Chmod",
		"Timestamps/Chtimes":              "ModTime after Chtimes",
	})
}

func TestSuiteFailsCleanlyWhenTestDirCannotBeCreated(t *testing.T) {
	f := newFaultFS(t)
	f.fail = failOps("MkdirAll")
	root := runSuite(t, f)
	if !strings.Contains(root.errorText(), "failed to create test directory") || len(root.children) != 0 {
		t.Errorf("errors %q, %d subtests ran", root.errorText(), len(root.children))
	}
}

func TestSuiteWithoutSymlinkFileSystem(t *testing.T) {
	plain := struct{ absfs.FileSystem }{newMemFS(t)}
	root := runSuite(t, plain)
	c := root.mustFind(t, "Symlinks")
	if !c.skipped || !strings.Contains(c.skipMsg, "SymlinkFileSystem") {
		t.Errorf("Symlinks skipped=%v msg=%q", c.skipped, c.skipMsg)
	}
}

func TestQuickCheckFailures(t *testing.T) {
	tests := []struct {
		name string
		fs   func(*faultFS)
		want string
	}{
		{"pass", func(*faultFS) {}, ""},
		{"mkdir", func(f *faultFS) { f.fail = failOps("MkdirAll") }, "failed to create test directory"},
		{"create", func(f *faultFS) { f.fail = failOps("Create") }, "Create"},
		{"open", func(f *faultFS) { f.fail = failOps("Open") }, "Open"},
		{"remove", func(f *faultFS) { f.fail = failOps("Remove") }, "Remove"},
		{"lie", func(f *faultFS) { f.lie = true }, "content mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFaultFS(t)
			tt.fs(f)
			s := &Suite{FS: f}
			root := runFake(func(t tb) { s.quickCheck(t) })
			root.assertNoPanic(t)
			if tt.want == "" {
				if root.failed {
					t.Errorf("failed: %s", root.errorText())
				}
				return
			}
			if !strings.Contains(root.errorText(), tt.want) {
				t.Errorf("errors %q lack %q", root.errorText(), tt.want)
			}
		})
	}
}

// TestQuickCheckFileLingers covers a Remove that reports success but leaves
// the file behind.
func TestQuickCheckFileLingers(t *testing.T) {
	f := &lingerFS{faultFS: newFaultFS(t)}
	s := &Suite{FS: f}
	root := runFake(func(t tb) { s.quickCheck(t) })
	if !strings.Contains(root.errorText(), "should not exist after Remove") {
		t.Errorf("errors %q", root.errorText())
	}
}

type lingerFS struct{ *faultFS }

func (lingerFS) Remove(string) error { return nil }

func TestHelpersRejectNilResults(t *testing.T) {
	f := &nilFS{faultFS: newFaultFS(t)}
	s := &Suite{FS: f}
	for name, fn := range map[string]func(t tb){
		"writeFile": func(t tb) { s.writeFile(t, "/x", nil) },
		"readFile":  func(t tb) { s.readFile(t, "/x") },
		"stat":      func(t tb) { s.stat(t, "/x") },
		"lstat":     func(t tb) { lstat(t, f, "/x") },
	} {
		root := runFake(fn)
		root.assertNoPanic(t)
		if !strings.Contains(root.errorText(), "nil") {
			t.Errorf("%s: errors %q do not report the nil result", name, root.errorText())
		}
	}
}

// nilFS returns nil results with nil errors, which no correct filesystem does.
type nilFS struct{ *faultFS }

func (nilFS) Create(string) (absfs.File, error) { return nil, nil }
func (nilFS) Open(string) (absfs.File, error)   { return nil, nil }
func (nilFS) Stat(string) (os.FileInfo, error)  { return nil, nil }
func (nilFS) Lstat(string) (os.FileInfo, error) { return nil, nil }

func TestWriteFileReportsWriteAndCloseErrors(t *testing.T) {
	for _, mode := range []string{"write", "close"} {
		f := &badFileFS{faultFS: newFaultFS(t), mode: mode}
		s := &Suite{FS: f}
		root := runFake(func(t tb) { s.writeFile(t, "/x", []byte("data")) })
		if !strings.Contains(root.errorText(), map[string]string{"write": "Write", "close": "Close"}[mode]) {
			t.Errorf("%s: errors %q", mode, root.errorText())
		}
	}
	f := &badFileFS{faultFS: newFaultFS(t), mode: "read"}
	if err := f.FileSystem.MkdirAll("/d", 0755); err != nil {
		t.Fatal(err)
	}
	s := &Suite{FS: f}
	root := runFake(func(t tb) { s.readFile(t, "/d") })
	if !strings.Contains(root.errorText(), "Read") {
		t.Errorf("read: errors %q", root.errorText())
	}
}

// badFileFS returns files whose Write, Close, or Read fails.
type badFileFS struct {
	*faultFS
	mode string
}

func (b *badFileFS) Create(name string) (absfs.File, error) {
	f, err := b.FileSystem.Create(name)
	return &badFile{File: f, mode: b.mode}, err
}

func (b *badFileFS) Open(name string) (absfs.File, error) {
	f, err := b.FileSystem.Open(name)
	return &badFile{File: f, mode: b.mode}, err
}

type badFile struct {
	absfs.File
	mode string
}

func (b *badFile) Write(p []byte) (int, error) {
	if b.mode == "write" {
		return 0, errFault
	}
	return b.File.Write(p)
}

func (b *badFile) Close() error {
	b.File.Close()
	if b.mode == "close" {
		return errFault
	}
	return nil
}

func (b *badFile) Read(p []byte) (int, error) {
	if b.mode == "read" {
		return 0, errFault
	}
	return b.File.Read(p)
}
