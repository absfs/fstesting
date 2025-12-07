package fstesting

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/absfs/absfs"
)

// Suite provides baseline tests for any absfs.FileSystem implementation.
// It tests core operations that all implementations should support.
type Suite struct {
	// FS is the filesystem to test. Required.
	FS absfs.FileSystem

	// Features describes which optional features the FS supports.
	// Tests for unsupported features are skipped.
	Features Features

	// TestDir is the directory within FS to run tests in.
	// If empty, uses FS.TempDir().
	TestDir string

	// KeepTestDir if true, doesn't clean up the test directory after tests.
	// Useful for debugging.
	KeepTestDir bool
}

// Run executes all applicable tests based on the configured features.
func (s *Suite) Run(t *testing.T) {
	t.Helper()

	// Set up test directory
	testDir := s.TestDir
	if testDir == "" {
		testDir = s.FS.TempDir()
	}
	testDir = path.Join(testDir, fmt.Sprintf("fstesting_%d", time.Now().UnixNano()))

	if err := s.FS.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	if !s.KeepTestDir {
		t.Cleanup(func() {
			s.FS.RemoveAll(testDir)
		})
	}

	// Run test groups
	t.Run("FileOperations", func(t *testing.T) {
		s.testFileOperations(t, testDir)
	})

	t.Run("DirectoryOperations", func(t *testing.T) {
		s.testDirectoryOperations(t, testDir)
	})

	t.Run("PathHandling", func(t *testing.T) {
		s.testPathHandling(t, testDir)
	})

	t.Run("ErrorSemantics", func(t *testing.T) {
		s.testErrorSemantics(t, testDir)
	})

	if s.Features.Symlinks {
		t.Run("Symlinks", func(t *testing.T) {
			s.testSymlinks(t, testDir)
		})
	}

	if s.Features.Permissions {
		t.Run("Permissions", func(t *testing.T) {
			s.testPermissions(t, testDir)
		})
	}

	if s.Features.Timestamps {
		t.Run("Timestamps", func(t *testing.T) {
			s.testTimestamps(t, testDir)
		})
	}
}

// testFileOperations tests basic file CRUD operations.
func (s *Suite) testFileOperations(t *testing.T, testDir string) {
	t.Helper()

	t.Run("CreateAndRead", func(t *testing.T) {
		filePath := path.Join(testDir, "create_test.txt")
		content := []byte("hello, world")

		// Create file
		f, err := s.FS.Create(filePath)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		n, err := f.Write(content)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		if n != len(content) {
			t.Errorf("Write returned %d, want %d", n, len(content))
		}

		if err := f.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		// Read back
		f, err = s.FS.Open(filePath)
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer f.Close()

		got, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}

		if !bytes.Equal(got, content) {
			t.Errorf("content mismatch: got %q, want %q", got, content)
		}
	})

	t.Run("OpenFile", func(t *testing.T) {
		filePath := path.Join(testDir, "openfile_test.txt")

		// O_CREATE | O_EXCL should create new file
		f, err := s.FS.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatalf("OpenFile O_CREATE|O_EXCL failed: %v", err)
		}
		f.Close()

		// O_EXCL should fail if file exists
		_, err = s.FS.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			t.Error("OpenFile O_EXCL should fail for existing file")
		}
	})

	t.Run("Truncate", func(t *testing.T) {
		filePath := path.Join(testDir, "truncate_test.txt")
		content := []byte("hello, world")

		// Create file with content
		f, err := s.FS.Create(filePath)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		f.Write(content)
		f.Close()

		// Truncate to 5 bytes
		if err := s.FS.Truncate(filePath, 5); err != nil {
			t.Fatalf("Truncate failed: %v", err)
		}

		// Verify size
		info, err := s.FS.Stat(filePath)
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}
		if info.Size() != 5 {
			t.Errorf("size after truncate: got %d, want 5", info.Size())
		}
	})

	t.Run("Remove", func(t *testing.T) {
		filePath := path.Join(testDir, "remove_test.txt")

		// Create and remove
		f, _ := s.FS.Create(filePath)
		f.Close()

		if err := s.FS.Remove(filePath); err != nil {
			t.Fatalf("Remove failed: %v", err)
		}

		// Verify gone
		_, err := s.FS.Stat(filePath)
		if !os.IsNotExist(err) {
			t.Errorf("file should not exist after Remove, got err: %v", err)
		}
	})

	t.Run("Rename", func(t *testing.T) {
		oldPath := path.Join(testDir, "rename_old.txt")
		newPath := path.Join(testDir, "rename_new.txt")
		content := []byte("rename test content")

		// Create file
		f, _ := s.FS.Create(oldPath)
		f.Write(content)
		f.Close()

		// Rename
		if err := s.FS.Rename(oldPath, newPath); err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// Old path should not exist
		_, err := s.FS.Stat(oldPath)
		if !os.IsNotExist(err) {
			t.Error("old path should not exist after Rename")
		}

		// New path should exist with same content
		f, err = s.FS.Open(newPath)
		if err != nil {
			t.Fatalf("Open new path failed: %v", err)
		}
		defer f.Close()

		got, _ := io.ReadAll(f)
		if !bytes.Equal(got, content) {
			t.Errorf("content mismatch after rename: got %q, want %q", got, content)
		}
	})

	t.Run("Stat", func(t *testing.T) {
		filePath := path.Join(testDir, "stat_test.txt")
		content := []byte("stat test")

		f, _ := s.FS.Create(filePath)
		f.Write(content)
		f.Close()

		info, err := s.FS.Stat(filePath)
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}

		if info.Name() != "stat_test.txt" {
			t.Errorf("Name: got %q, want %q", info.Name(), "stat_test.txt")
		}

		if info.Size() != int64(len(content)) {
			t.Errorf("Size: got %d, want %d", info.Size(), len(content))
		}

		if info.IsDir() {
			t.Error("IsDir should be false for file")
		}
	})
}

// testDirectoryOperations tests directory-related operations.
func (s *Suite) testDirectoryOperations(t *testing.T, testDir string) {
	t.Helper()

	t.Run("Mkdir", func(t *testing.T) {
		dirPath := path.Join(testDir, "mkdir_test")

		if err := s.FS.Mkdir(dirPath, 0755); err != nil {
			t.Fatalf("Mkdir failed: %v", err)
		}

		info, err := s.FS.Stat(dirPath)
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}

		if !info.IsDir() {
			t.Error("created path should be a directory")
		}
	})

	t.Run("MkdirAll", func(t *testing.T) {
		dirPath := path.Join(testDir, "mkdirall", "nested", "path")

		if err := s.FS.MkdirAll(dirPath, 0755); err != nil {
			t.Fatalf("MkdirAll failed: %v", err)
		}

		info, err := s.FS.Stat(dirPath)
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}

		if !info.IsDir() {
			t.Error("created path should be a directory")
		}
	})

	t.Run("RemoveAll", func(t *testing.T) {
		base := path.Join(testDir, "removeall_test")
		nested := path.Join(base, "nested")
		file := path.Join(nested, "file.txt")

		s.FS.MkdirAll(nested, 0755)
		f, _ := s.FS.Create(file)
		f.Close()

		if err := s.FS.RemoveAll(base); err != nil {
			t.Fatalf("RemoveAll failed: %v", err)
		}

		_, err := s.FS.Stat(base)
		if !os.IsNotExist(err) {
			t.Error("directory should not exist after RemoveAll")
		}
	})

	t.Run("ReadDir", func(t *testing.T) {
		base := path.Join(testDir, "readdir_test")
		s.FS.Mkdir(base, 0755)

		// Create some files
		names := []string{"a.txt", "b.txt", "c.txt"}
		for _, name := range names {
			f, _ := s.FS.Create(path.Join(base, name))
			f.Close()
		}

		// Create a subdirectory
		s.FS.Mkdir(path.Join(base, "subdir"), 0755)

		// Read directory
		f, err := s.FS.Open(base)
		if err != nil {
			t.Fatalf("Open directory failed: %v", err)
		}
		defer f.Close()

		entries, err := f.Readdir(-1)
		if err != nil {
			t.Fatalf("Readdir failed: %v", err)
		}

		if len(entries) != 4 { // 3 files + 1 subdir
			t.Errorf("Readdir returned %d entries, want 4", len(entries))
		}
	})
}

// testPathHandling tests path edge cases.
func (s *Suite) testPathHandling(t *testing.T, testDir string) {
	t.Helper()

	t.Run("DotPaths", func(t *testing.T) {
		// Current directory reference
		base := path.Join(testDir, "dotpaths")
		s.FS.Mkdir(base, 0755)

		filePath := path.Join(base, ".", "file.txt")
		f, err := s.FS.Create(filePath)
		if err != nil {
			t.Fatalf("Create with . in path failed: %v", err)
		}
		f.Close()
	})

	t.Run("TrailingSlash", func(t *testing.T) {
		base := path.Join(testDir, "trailingslash")
		s.FS.Mkdir(base, 0755)

		// Stat directory with trailing slash
		_, err := s.FS.Stat(base + string(s.FS.Separator()))
		if err != nil {
			t.Errorf("Stat with trailing slash failed: %v", err)
		}
	})

	t.Run("SpecialCharacters", func(t *testing.T) {
		base := path.Join(testDir, "specialchars")
		s.FS.Mkdir(base, 0755)

		// Test various special characters in filenames
		names := []string{
			"spaces in name.txt",
			"file-with-dashes.txt",
			"file_with_underscores.txt",
			"file.multiple.dots.txt",
		}

		for _, name := range names {
			filePath := path.Join(base, name)
			f, err := s.FS.Create(filePath)
			if err != nil {
				t.Errorf("Create %q failed: %v", name, err)
				continue
			}
			f.Close()

			_, err = s.FS.Stat(filePath)
			if err != nil {
				t.Errorf("Stat %q failed: %v", name, err)
			}
		}
	})

	t.Run("Unicode", func(t *testing.T) {
		base := path.Join(testDir, "unicode")
		s.FS.Mkdir(base, 0755)

		names := []string{
			"日本語.txt",
			"émoji🎉.txt",
			"中文文件.txt",
		}

		for _, name := range names {
			filePath := path.Join(base, name)
			f, err := s.FS.Create(filePath)
			if err != nil {
				t.Errorf("Create unicode %q failed: %v", name, err)
				continue
			}
			f.Close()

			_, err = s.FS.Stat(filePath)
			if err != nil {
				t.Errorf("Stat unicode %q failed: %v", name, err)
			}
		}
	})
}

// testErrorSemantics tests that errors match os package behavior.
func (s *Suite) testErrorSemantics(t *testing.T, testDir string) {
	t.Helper()

	t.Run("NotExist", func(t *testing.T) {
		_, err := s.FS.Stat(path.Join(testDir, "nonexistent"))
		if !os.IsNotExist(err) {
			t.Errorf("Stat nonexistent: expected os.IsNotExist, got %v", err)
		}

		_, err = s.FS.Open(path.Join(testDir, "nonexistent"))
		if !os.IsNotExist(err) {
			t.Errorf("Open nonexistent: expected os.IsNotExist, got %v", err)
		}
	})

	t.Run("Exist", func(t *testing.T) {
		dirPath := path.Join(testDir, "exist_test")
		s.FS.Mkdir(dirPath, 0755)

		err := s.FS.Mkdir(dirPath, 0755)
		if !os.IsExist(err) {
			t.Errorf("Mkdir existing: expected os.IsExist, got %v", err)
		}
	})

	t.Run("IsDir", func(t *testing.T) {
		dirPath := path.Join(testDir, "isdir_test")
		s.FS.Mkdir(dirPath, 0755)

		// Try to open directory as file for writing
		_, err := s.FS.OpenFile(dirPath, os.O_WRONLY, 0644)
		if err == nil {
			t.Error("OpenFile directory for writing should fail")
		}
	})

	t.Run("NotDir", func(t *testing.T) {
		filePath := path.Join(testDir, "notdir_test.txt")
		f, _ := s.FS.Create(filePath)
		f.Close()

		// Try to read file as directory
		f, err := s.FS.Open(filePath)
		if err != nil {
			t.Fatalf("Open file failed: %v", err)
		}

		_, err = f.Readdir(-1)
		f.Close()
		if err == nil {
			t.Error("Readdir on file should fail")
		}
	})
}

// testSymlinks tests symbolic link operations.
func (s *Suite) testSymlinks(t *testing.T, testDir string) {
	t.Helper()

	// Check if FS implements SymlinkFileSystem
	sfs, ok := s.FS.(absfs.SymlinkFileSystem)
	if !ok {
		t.Skip("filesystem does not implement SymlinkFileSystem")
	}

	t.Run("CreateAndRead", func(t *testing.T) {
		target := path.Join(testDir, "symlink_target.txt")
		link := path.Join(testDir, "symlink_link")

		// Create target
		f, _ := s.FS.Create(target)
		f.Write([]byte("symlink target content"))
		f.Close()

		// Create symlink
		if err := sfs.Symlink(target, link); err != nil {
			t.Fatalf("Symlink failed: %v", err)
		}

		// Read link
		got, err := sfs.Readlink(link)
		if err != nil {
			t.Fatalf("Readlink failed: %v", err)
		}

		if got != target {
			t.Errorf("Readlink: got %q, want %q", got, target)
		}
	})

	t.Run("Lstat", func(t *testing.T) {
		target := path.Join(testDir, "lstat_target.txt")
		link := path.Join(testDir, "lstat_link")

		f, _ := s.FS.Create(target)
		f.Close()
		sfs.Symlink(target, link)

		// Stat follows symlink
		info, _ := s.FS.Stat(link)
		if info.Mode()&os.ModeSymlink != 0 {
			t.Error("Stat should follow symlink")
		}

		// Lstat does not follow symlink
		info, err := sfs.Lstat(link)
		if err != nil {
			t.Fatalf("Lstat failed: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("Lstat should return symlink info")
		}
	})
}

// testPermissions tests permission-related operations.
func (s *Suite) testPermissions(t *testing.T, testDir string) {
	t.Helper()

	t.Run("Chmod", func(t *testing.T) {
		filePath := path.Join(testDir, "chmod_test.txt")
		f, _ := s.FS.Create(filePath)
		f.Close()

		if err := s.FS.Chmod(filePath, 0600); err != nil {
			t.Fatalf("Chmod failed: %v", err)
		}

		info, _ := s.FS.Stat(filePath)
		// Mask out non-permission bits
		got := info.Mode().Perm()
		if got != 0600 {
			t.Errorf("mode after Chmod: got %o, want 0600", got)
		}
	})
}

// testTimestamps tests timestamp-related operations.
func (s *Suite) testTimestamps(t *testing.T, testDir string) {
	t.Helper()

	t.Run("Chtimes", func(t *testing.T) {
		filePath := path.Join(testDir, "chtimes_test.txt")
		f, _ := s.FS.Create(filePath)
		f.Close()

		atime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		mtime := time.Date(2021, 6, 15, 12, 0, 0, 0, time.UTC)

		if err := s.FS.Chtimes(filePath, atime, mtime); err != nil {
			t.Fatalf("Chtimes failed: %v", err)
		}

		info, _ := s.FS.Stat(filePath)
		got := info.ModTime()

		// Allow 1 second tolerance for filesystems with low time resolution
		diff := got.Sub(mtime)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("ModTime after Chtimes: got %v, want %v", got, mtime)
		}
	})
}

// QuickCheck runs a minimal set of tests to verify basic functionality.
// Useful for quick sanity checks.
func (s *Suite) QuickCheck(t *testing.T) {
	t.Helper()

	testDir := path.Join(s.FS.TempDir(), fmt.Sprintf("fstesting_quick_%d", time.Now().UnixNano()))
	if err := s.FS.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	defer s.FS.RemoveAll(testDir)

	// Test create/read/delete cycle
	filePath := path.Join(testDir, "quickcheck.txt")
	content := []byte("quick check content")

	f, err := s.FS.Create(filePath)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.Write(content)
	f.Close()

	f, err = s.FS.Open(filePath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, _ := io.ReadAll(f)
	f.Close()

	if !bytes.Equal(got, content) {
		t.Fatalf("content mismatch: got %q, want %q", got, content)
	}

	if err := s.FS.Remove(filePath); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := s.FS.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("file should not exist after Remove")
	}
}

// RunWithSkips is like Run but allows specifying test names to skip.
// Useful when an implementation has known limitations.
func (s *Suite) RunWithSkips(t *testing.T, skips ...string) {
	t.Helper()

	skipMap := make(map[string]bool)
	for _, skip := range skips {
		skipMap[strings.ToLower(skip)] = true
	}

	// Override t.Run to check skips
	origRun := t.Run
	_ = origRun // Note: Can't actually override, but document the pattern

	// For now, just run normally - implementations should use t.Skip()
	// in their own test files for known limitations
	s.Run(t)
}
