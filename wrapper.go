package fstesting

import (
	"bytes"
	"io"
	"path/filepath"
	"testing"

	"github.com/absfs/absfs"
)

// WrapperSuite tests that a wrapper correctly delegates operations
// and maintains data integrity through any transformations.
type WrapperSuite struct {
	// Factory creates a wrapper around the given base filesystem.
	// Required.
	Factory func(base absfs.FileSystem) (absfs.FileSystem, error)

	// BaseFS is the underlying filesystem to wrap.
	// If nil, tests will create their own in-memory base.
	BaseFS absfs.FileSystem

	// Name is a descriptive name for the wrapper (e.g., "compressfs").
	Name string

	// TransformsData indicates the wrapper modifies file contents
	// (e.g., compression, encryption). If true, data written through
	// the wrapper may not be directly readable from the base.
	TransformsData bool

	// TransformsMeta indicates the wrapper modifies metadata
	// (e.g., permissions, timestamps).
	TransformsMeta bool

	// ReadOnly indicates the wrapper blocks all write operations.
	ReadOnly bool

	// TestDir is the directory to run tests in.
	TestDir string
}

// Run executes all wrapper tests.
func (s *WrapperSuite) Run(t *testing.T) {
	t.Helper()

	if s.BaseFS == nil {
		t.Fatal("WrapperSuite requires BaseFS to be set")
	}

	wrapper, err := s.Factory(s.BaseFS)
	if err != nil {
		t.Fatalf("Factory failed: %v", err)
	}

	testDir := s.TestDir
	if testDir == "" {
		testDir = wrapper.TempDir()
	}
	testDir = filepath.Join(testDir, "wrapper_test")

	if err := wrapper.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	t.Cleanup(func() {
		wrapper.RemoveAll(testDir)
	})

	t.Run("Passthrough", func(t *testing.T) {
		s.testPassthrough(t, wrapper, testDir)
	})

	t.Run("DataIntegrity", func(t *testing.T) {
		s.testDataIntegrity(t, wrapper, testDir)
	})

	if s.ReadOnly {
		t.Run("WriteBlocking", func(t *testing.T) {
			s.testWriteBlocking(t, wrapper, testDir)
		})
	}

	if s.TransformsData {
		t.Run("TransformRoundtrip", func(t *testing.T) {
			s.testTransformRoundtrip(t, wrapper, testDir)
		})
	}
}

// testPassthrough verifies basic operations work through the wrapper.
func (s *WrapperSuite) testPassthrough(t *testing.T, wrapper absfs.FileSystem, testDir string) {
	t.Helper()

	if s.ReadOnly {
		t.Skip("skipping passthrough test for read-only wrapper")
	}

	// Create file through wrapper
	path := filepath.Join(testDir, "passthrough.txt")
	content := []byte("passthrough test content")

	f, err := wrapper.Create(path)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	f.Write(content)
	f.Close()

	// Read back through wrapper
	f, err = wrapper.Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	got, _ := io.ReadAll(f)
	f.Close()

	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %d bytes, want %d bytes", len(got), len(content))
	}

	// Stat through wrapper
	info, err := wrapper.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.IsDir() {
		t.Error("file should not be a directory")
	}

	// Remove through wrapper
	if err := wrapper.Remove(path); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
}

// testDataIntegrity verifies data survives write/read cycle.
func (s *WrapperSuite) testDataIntegrity(t *testing.T, wrapper absfs.FileSystem, testDir string) {
	t.Helper()

	if s.ReadOnly {
		t.Skip("skipping data integrity test for read-only wrapper")
	}

	testCases := []struct {
		name    string
		content []byte
	}{
		{"empty", []byte{}},
		{"small", []byte("hello")},
		{"binary", []byte{0x00, 0xFF, 0x00, 0xFF}},
		{"unicode", []byte("日本語テスト🎉")},
		{"large", bytes.Repeat([]byte("x"), 1<<16)}, // 64KB
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(testDir, "integrity_"+tc.name+".bin")

			// Write
			f, err := wrapper.Create(path)
			if err != nil {
				t.Fatalf("Create failed: %v", err)
			}
			n, err := f.Write(tc.content)
			if err != nil {
				f.Close()
				t.Fatalf("Write failed: %v", err)
			}
			if n != len(tc.content) {
				t.Errorf("Write returned %d, want %d", n, len(tc.content))
			}
			f.Close()

			// Read
			f, err = wrapper.Open(path)
			if err != nil {
				t.Fatalf("Open failed: %v", err)
			}
			got, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				t.Fatalf("ReadAll failed: %v", err)
			}

			if !bytes.Equal(got, tc.content) {
				t.Errorf("content mismatch: got %d bytes, want %d bytes", len(got), len(tc.content))
			}

			// Clean up
			wrapper.Remove(path)
		})
	}
}

// testWriteBlocking verifies that write operations fail for read-only wrappers.
func (s *WrapperSuite) testWriteBlocking(t *testing.T, wrapper absfs.FileSystem, testDir string) {
	t.Helper()

	path := filepath.Join(testDir, "writeblock.txt")

	// Create should fail
	_, err := wrapper.Create(path)
	if err == nil {
		t.Error("Create should fail for read-only wrapper")
	}

	// Mkdir should fail
	err = wrapper.Mkdir(filepath.Join(testDir, "newdir"), 0755)
	if err == nil {
		t.Error("Mkdir should fail for read-only wrapper")
	}

	// Remove should fail (if there's something to remove)
	err = wrapper.Remove(testDir)
	if err == nil {
		t.Error("Remove should fail for read-only wrapper")
	}
}

// testTransformRoundtrip verifies data survives transformation.
func (s *WrapperSuite) testTransformRoundtrip(t *testing.T, wrapper absfs.FileSystem, testDir string) {
	t.Helper()

	path := filepath.Join(testDir, "transform.bin")
	content := bytes.Repeat([]byte("compressible data pattern "), 1000)

	// Write through wrapper
	f, err := wrapper.Create(path)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	f.Write(content)
	f.Close()

	// Read back through wrapper - should get original data
	f, err = wrapper.Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	got, _ := io.ReadAll(f)
	f.Close()

	if !bytes.Equal(got, content) {
		t.Errorf("roundtrip failed: got %d bytes, want %d bytes", len(got), len(content))
	}

	// Clean up
	wrapper.Remove(path)
}

// FuzzWrapperRoundtrip fuzz tests data integrity through a wrapper.
func FuzzWrapperRoundtrip(f *testing.F, factory func(absfs.FileSystem) (absfs.FileSystem, error), base absfs.FileSystem, testDir string) {
	f.Add([]byte("hello"))
	f.Add([]byte{0x00, 0xFF})
	f.Add(make([]byte, 4096))
	f.Add(bytes.Repeat([]byte("pattern"), 1000))

	wrapper, err := factory(base)
	if err != nil {
		f.Fatalf("Factory failed: %v", err)
	}

	wrapper.MkdirAll(testDir, 0755)

	counter := 0
	f.Fuzz(func(t *testing.T, data []byte) {
		counter++
		path := filepath.Join(testDir, "fuzz_wrapper", string(rune('a'+counter%26))+".bin")
		wrapper.MkdirAll(filepath.Dir(path), 0755)

		// Write
		file, err := wrapper.Create(path)
		if err != nil {
			return
		}
		file.Write(data)
		file.Close()

		// Read back
		file, err = wrapper.Open(path)
		if err != nil {
			t.Fatalf("Open failed after successful Create: %v", err)
		}
		got, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}

		if !bytes.Equal(got, data) {
			t.Errorf("roundtrip failed: wrote %d bytes, got %d bytes", len(data), len(got))
		}

		wrapper.Remove(path)
	})
}
