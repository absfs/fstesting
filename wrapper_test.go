package fstesting

import (
	"errors"
	"strings"
	"testing"

	"github.com/absfs/absfs"
)

func identity(base absfs.FileSystem) (absfs.FileSystem, error) { return base, nil }

func TestWrapperSuitePassthrough(t *testing.T) {
	s := &WrapperSuite{
		Factory:        identity,
		BaseFS:         newMemFS(t),
		Name:           "identity",
		TransformsData: true,
	}
	s.Run(t)
}

func TestWrapperSuiteReadOnly(t *testing.T) {
	base := newFaultFS(t)
	base.fail = failOps("Create", "Mkdir", "Remove")
	s := &WrapperSuite{Factory: identity, BaseFS: base, ReadOnly: true}
	root := runFake(func(t tb) { s.run(t) })
	root.assertNoPanic(t)
	if root.failed {
		t.Fatalf("read-only wrapper failed: %v", root.all())
	}
	for _, name := range []string{"Passthrough", "DataIntegrity"} {
		if !root.mustFind(t, name).skipped {
			t.Errorf("%s not skipped for a read-only wrapper", name)
		}
	}
	if root.mustFind(t, "WriteBlocking").skipped {
		t.Error("WriteBlocking skipped")
	}
}

func TestWrapperSuiteSetupFailures(t *testing.T) {
	tests := []struct {
		name string
		s    func(*testing.T) *WrapperSuite
		want string
	}{
		{"no base", func(*testing.T) *WrapperSuite { return &WrapperSuite{Factory: identity} }, "requires BaseFS"},
		{"factory error", func(t *testing.T) *WrapperSuite {
			return &WrapperSuite{
				Factory: func(absfs.FileSystem) (absfs.FileSystem, error) { return nil, errors.New("no wrapper") },
				BaseFS:  newMemFS(t),
			}
		}, "Factory failed"},
		{"mkdir", func(t *testing.T) *WrapperSuite {
			f := newFaultFS(t)
			f.fail = failOps("MkdirAll")
			return &WrapperSuite{Factory: identity, BaseFS: f, TestDir: "/w"}
		}, "failed to create test directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.s(t)
			root := runFake(func(t tb) { s.run(t) })
			root.assertNoPanic(t)
			if !strings.Contains(root.errorText(), tt.want) {
				t.Errorf("errors %q lack %q", root.errorText(), tt.want)
			}
		})
	}
}

func TestWrapperSuiteFailures(t *testing.T) {
	tests := []struct {
		name     string
		readOnly bool
		base     func(*testing.T) absfs.FileSystem
		want     map[string]string
	}{
		{"create", false, func(t *testing.T) absfs.FileSystem {
			f := newFaultFS(t)
			f.fail = failOps("Create")
			return f
		}, map[string]string{
			"Passthrough":         "Create failed",
			"DataIntegrity/small": "Create failed",
			"TransformRoundtrip":  "Create failed",
		}},
		{"open", false, func(t *testing.T) absfs.FileSystem {
			f := newFaultFS(t)
			f.fail = failOps("Open")
			return f
		}, map[string]string{
			"Passthrough":         "Open failed",
			"DataIntegrity/small": "Open failed",
			"TransformRoundtrip":  "Open failed",
		}},
		{"stat", false, func(t *testing.T) absfs.FileSystem {
			f := newFaultFS(t)
			f.fail = failOps("Stat")
			return f
		}, map[string]string{"Passthrough": "Stat failed"}},
		{"remove", false, func(t *testing.T) absfs.FileSystem {
			f := newFaultFS(t)
			f.fail = failOps("Remove")
			return f
		}, map[string]string{"Passthrough": "Remove failed"}},
		{"write", false, func(t *testing.T) absfs.FileSystem {
			return &badFileFS{faultFS: newFaultFS(t), mode: "write"}
		}, map[string]string{"DataIntegrity/small": "Write failed"}},
		{"read", false, func(t *testing.T) absfs.FileSystem {
			return &badFileFS{faultFS: newFaultFS(t), mode: "read"}
		}, map[string]string{"DataIntegrity/small": "ReadAll failed"}},
		{"lie", false, func(t *testing.T) absfs.FileSystem {
			f := newFaultFS(t)
			f.lie = true
			return f
		}, map[string]string{
			"Passthrough":         "content mismatch",
			"DataIntegrity/small": "Write returned",
			"TransformRoundtrip":  "roundtrip failed",
		}},
		{"read-only that writes", true, func(t *testing.T) absfs.FileSystem {
			f := newFaultFS(t)
			f.lie = true
			return f
		}, map[string]string{"WriteBlocking": "should fail for read-only wrapper"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &WrapperSuite{
				Factory:        identity,
				BaseFS:         tt.base(t),
				ReadOnly:       tt.readOnly,
				TransformsData: true,
			}
			root := runFake(func(t tb) { s.run(t) })
			root.assertNoPanic(t)
			assertFailed(t, root, tt.want)
		})
	}
}
