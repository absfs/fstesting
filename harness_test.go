package fstesting

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/absfs/memfs"
)

// fakeT is a recording implementation of tb. It runs each test body on its
// own goroutine, as the testing package does, so Fatal and Skip can end the
// body with runtime.Goexit. A panic in a body is recovered and recorded, so a
// test can assert that a suite failed cleanly instead of crashing.
type fakeT struct {
	name string

	mu       sync.Mutex
	failed   bool
	skipped  bool
	panicked string
	errors   []string
	skipMsg  string
	logs     []string
	cleanups []func()
	children []*fakeT
}

var _ tb = (*fakeT)(nil)

func (f *fakeT) Helper()      {}
func (f *fakeT) Name() string { return f.name }

func (f *fakeT) fail(msg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed = true
	f.errors = append(f.errors, msg)
}

func (f *fakeT) Error(args ...any)                 { f.fail(fmt.Sprint(args...)) }
func (f *fakeT) Errorf(format string, args ...any) { f.fail(fmt.Sprintf(format, args...)) }
func (f *fakeT) Fatal(args ...any)                 { f.fail(fmt.Sprint(args...)); runtime.Goexit() }
func (f *fakeT) Fatalf(format string, args ...any) {
	f.fail(fmt.Sprintf(format, args...))
	runtime.Goexit()
}

func (f *fakeT) Skip(args ...any) {
	f.mu.Lock()
	f.skipped = true
	f.skipMsg = fmt.Sprint(args...)
	f.mu.Unlock()
	runtime.Goexit()
}

func (f *fakeT) Logf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, fmt.Sprintf(format, args...))
}

func (f *fakeT) Cleanup(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanups = append(f.cleanups, fn)
}

func (f *fakeT) Run(name string, fn func(t tb)) bool {
	c := &fakeT{name: f.name + "/" + name}
	f.mu.Lock()
	f.children = append(f.children, c)
	f.mu.Unlock()
	c.exec(fn)
	if c.failed {
		f.mu.Lock()
		f.failed = true
		f.mu.Unlock()
	}
	return !c.failed
}

// exec runs fn as the body of f, then f's cleanups in reverse order.
func (f *fakeT) exec(fn func(t tb)) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			for i := len(f.cleanups) - 1; i >= 0; i-- {
				f.cleanups[i]()
			}
		}()
		defer func() {
			if r := recover(); r != nil {
				f.mu.Lock()
				f.failed = true
				f.panicked = fmt.Sprint(r)
				f.mu.Unlock()
			}
		}()
		fn(f)
	}()
	<-done
}

// runFake runs fn against a fresh root fakeT and returns it.
func runFake(fn func(t tb)) *fakeT {
	root := &fakeT{name: "root"}
	root.exec(fn)
	return root
}

// all returns f and every descendant, depth first.
func (f *fakeT) all() []*fakeT {
	out := []*fakeT{f}
	for _, c := range f.children {
		out = append(out, c.all()...)
	}
	return out
}

// find returns the descendant whose path below f is rel, or nil.
func (f *fakeT) find(rel string) *fakeT {
	want := f.name + "/" + rel
	for _, c := range f.all() {
		if c.name == want {
			return c
		}
	}
	return nil
}

// mustFind is find that fails t when rel is missing.
func (f *fakeT) mustFind(t *testing.T, rel string) *fakeT {
	t.Helper()
	c := f.find(rel)
	if c == nil {
		t.Fatalf("subtest %q did not run", rel)
	}
	return c
}

// assertNoPanic fails t for every recorded panic under f.
func (f *fakeT) assertNoPanic(t *testing.T) {
	t.Helper()
	for _, c := range f.all() {
		if c.panicked != "" {
			t.Errorf("%s panicked: %s", c.name, c.panicked)
		}
	}
}

// errorText joins f's recorded failure messages.
func (f *fakeT) errorText() string { return strings.Join(f.errors, "\n") }

func newMemFS(t *testing.T) *memfs.FileSystem {
	t.Helper()
	m, err := memfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestFakeTPropagatesFailure checks the harness itself.
func TestFakeTPropagatesFailure(t *testing.T) {
	root := runFake(func(t tb) {
		t.Run("a", func(t tb) {
			t.Run("b", func(t tb) { t.Errorf("boom %d", 1) })
		})
		t.Run("c", func(t tb) { t.Error("plain") })
		t.Run("d", func(t tb) { panic("kaboom") })
	})
	if !root.failed {
		t.Fatal("failure did not propagate to the root")
	}
	if got := root.mustFind(t, "a/b").errorText(); got != "boom 1" {
		t.Errorf("a/b errors = %q", got)
	}
	if got := root.mustFind(t, "c").errorText(); got != "plain" {
		t.Errorf("c errors = %q", got)
	}
	if root.mustFind(t, "d").panicked != "kaboom" {
		t.Error("panic not recorded")
	}
	if root.find("missing") != nil {
		t.Error("find returned a subtest that never ran")
	}
}
