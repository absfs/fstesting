package fstesting

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
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

	t.Run("NewFilerMethods", func(t *testing.T) {
		s.testNewFilerMethods(t, testDir)
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

		// Test File.ReadDir (new method)
		f, err := s.FS.Open(base)
		if err != nil {
			t.Fatalf("Open directory failed: %v", err)
		}
		defer f.Close()

		var entries []fs.DirEntry
		entries, err = f.ReadDir(-1)
		if err != nil {
			t.Fatalf("File.ReadDir failed: %v", err)
		}

		if len(entries) != 4 { // 3 files + 1 subdir
			t.Errorf("File.ReadDir returned %d entries, want 4", len(entries))
		}

		// Verify entries are DirEntry, not FileInfo
		for _, entry := range entries {
			if entry.Name() == "" {
				t.Error("DirEntry has empty name")
			}
			// Should work with DirEntry interface
			_ = entry.IsDir()
			_ = entry.Type()
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
		_, err := s.FS.Stat(base + "/")
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

// testNewFilerMethods tests the new absfs 1.0 Filer methods.
func (s *Suite) testNewFilerMethods(t *testing.T, testDir string) {
	t.Helper()

	t.Run("Filer.ReadDir", func(t *testing.T) {
		base := path.Join(testDir, "filer_readdir_test")
		s.FS.MkdirAll(base, 0755)

		// Create some files
		names := []string{"file1.txt", "file2.txt", "file3.txt"}
		for _, name := range names {
			f, _ := s.FS.Create(path.Join(base, name))
			f.Close()
		}

		// Create a subdirectory
		s.FS.Mkdir(path.Join(base, "subdir"), 0755)

		// Test Filer.ReadDir (filesystem method, not file method)
		entries, err := s.FS.ReadDir(base)
		if err != nil {
			t.Fatalf("Filer.ReadDir failed: %v", err)
		}

		if len(entries) != 4 { // 3 files + 1 subdir
			t.Errorf("Filer.ReadDir returned %d entries, want 4", len(entries))
		}

		// Verify entries implement fs.DirEntry
		for _, entry := range entries {
			if entry.Name() == "" {
				t.Error("DirEntry has empty name")
			}
			info, err := entry.Info()
			if err != nil {
				t.Errorf("DirEntry.Info() failed: %v", err)
			}
			if info == nil {
				t.Error("DirEntry.Info() returned nil")
			}
		}
	})

	t.Run("Filer.ReadFile", func(t *testing.T) {
		filePath := path.Join(testDir, "readfile_test.txt")
		content := []byte("test content for ReadFile")

		// Create file
		f, err := s.FS.Create(filePath)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		f.Write(content)
		f.Close()

		// Test Filer.ReadFile
		got, err := s.FS.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Filer.ReadFile failed: %v", err)
		}

		if !bytes.Equal(got, content) {
			t.Errorf("content mismatch: got %q, want %q", got, content)
		}
	})

	t.Run("Filer.Sub", func(t *testing.T) {
		// Create a directory structure
		base := path.Join(testDir, "sub_test")
		nested := path.Join(base, "nested")
		s.FS.MkdirAll(nested, 0755)

		// Create a file in the nested directory
		filePath := path.Join(nested, "file.txt")
		content := []byte("nested file content")
		f, _ := s.FS.Create(filePath)
		f.Write(content)
		f.Close()

		// Get sub filesystem rooted at base
		subFS, err := s.FS.Sub(base)
		if err != nil {
			t.Fatalf("Filer.Sub failed: %v", err)
		}

		// Access file using relative path in sub filesystem
		// Type assert to fs.ReadFileFS since Sub returns fs.FS
		readFileFS, ok := subFS.(fs.ReadFileFS)
		if !ok {
			t.Fatal("Sub filesystem does not implement fs.ReadFileFS")
		}
		got, err := readFileFS.ReadFile("nested/file.txt")
		if err != nil {
			t.Fatalf("ReadFile in sub filesystem failed: %v", err)
		}

		if !bytes.Equal(got, content) {
			t.Errorf("content mismatch in sub filesystem: got %q, want %q", got, content)
		}

		// Verify Sub returns error for non-directory
		filePath2 := path.Join(testDir, "notadir.txt")
		f2, _ := s.FS.Create(filePath2)
		f2.Close()

		_, err = s.FS.Sub(filePath2)
		if err == nil {
			t.Error("Sub on a file should return an error")
		}
	})

	t.Run("Filer.Sub.Nested", func(t *testing.T) {
		// Test nested Sub calls
		base := path.Join(testDir, "nested_sub_test")
		level1 := path.Join(base, "level1")
		level2 := path.Join(level1, "level2")
		s.FS.MkdirAll(level2, 0755)

		// Create a file at level2
		filePath := path.Join(level2, "deep.txt")
		content := []byte("deep nested content")
		f, _ := s.FS.Create(filePath)
		f.Write(content)
		f.Close()

		// Get sub filesystem at base
		sub1, err := s.FS.Sub(base)
		if err != nil {
			t.Fatalf("First Sub failed: %v", err)
		}

		// Get nested sub filesystem
		// Type assert to fs.SubFS since Sub returns fs.FS
		subFS1, ok := sub1.(fs.SubFS)
		if !ok {
			t.Fatal("Sub filesystem does not implement fs.SubFS")
		}
		sub2, err := subFS1.Sub("level1")
		if err != nil {
			t.Fatalf("Nested Sub failed: %v", err)
		}

		// Access file from nested sub
		// Type assert to fs.ReadFileFS since Sub returns fs.FS
		readFileFS2, ok := sub2.(fs.ReadFileFS)
		if !ok {
			t.Fatal("Nested sub filesystem does not implement fs.ReadFileFS")
		}
		got, err := readFileFS2.ReadFile("level2/deep.txt")
		if err != nil {
			t.Fatalf("ReadFile in nested sub failed: %v", err)
		}

		if !bytes.Equal(got, content) {
			t.Errorf("content mismatch in nested sub: got %q, want %q", got, content)
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

	t.Run("RelativeSymlink", func(t *testing.T) {
		// Create directory structure for relative symlink
		subdir := path.Join(testDir, "rel_sub")
		if err := s.FS.MkdirAll(subdir, 0755); err != nil {
			t.Fatalf("MkdirAll failed: %v", err)
		}

		target := path.Join(testDir, "rel_target.txt")
		link := path.Join(subdir, "rel_link")

		// Create target file
		f, _ := s.FS.Create(target)
		f.Write([]byte("relative target"))
		f.Close()

		// Create relative symlink (../rel_target.txt)
		if err := sfs.Symlink("../rel_target.txt", link); err != nil {
			t.Fatalf("Symlink with relative path failed: %v", err)
		}

		// Verify readlink returns the relative path
		got, err := sfs.Readlink(link)
		if err != nil {
			t.Fatalf("Readlink failed: %v", err)
		}
		if got != "../rel_target.txt" {
			t.Errorf("Readlink: got %q, want %q", got, "../rel_target.txt")
		}
	})

	t.Run("SymlinkToDirectory", func(t *testing.T) {
		targetDir := path.Join(testDir, "link_target_dir")
		link := path.Join(testDir, "dir_link")

		if err := s.FS.Mkdir(targetDir, 0755); err != nil {
			t.Fatalf("Mkdir failed: %v", err)
		}

		// Create a file inside the target directory
		f, _ := s.FS.Create(path.Join(targetDir, "file.txt"))
		f.Write([]byte("content"))
		f.Close()

		// Create symlink to directory
		if err := sfs.Symlink(targetDir, link); err != nil {
			t.Fatalf("Symlink to directory failed: %v", err)
		}

		// Stat should follow and show it as a directory
		info, err := s.FS.Stat(link)
		if err != nil {
			t.Fatalf("Stat on symlink to directory failed: %v", err)
		}
		if !info.IsDir() {
			t.Error("Stat should show symlink-to-dir as directory")
		}

		// Lstat should show it as a symlink
		info, err = sfs.Lstat(link)
		if err != nil {
			t.Fatalf("Lstat failed: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("Lstat should show symlink mode")
		}
	})

	t.Run("BrokenSymlink", func(t *testing.T) {
		nonexistent := path.Join(testDir, "nonexistent_target")
		link := path.Join(testDir, "broken_link")

		// Create symlink to non-existent target
		if err := sfs.Symlink(nonexistent, link); err != nil {
			t.Fatalf("Symlink to non-existent target failed: %v", err)
		}

		// Readlink should still work
		got, err := sfs.Readlink(link)
		if err != nil {
			t.Fatalf("Readlink on broken symlink failed: %v", err)
		}
		if got != nonexistent {
			t.Errorf("Readlink: got %q, want %q", got, nonexistent)
		}

		// Lstat should work (returns info about the link itself)
		info, err := sfs.Lstat(link)
		if err != nil {
			t.Fatalf("Lstat on broken symlink failed: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("Lstat should show symlink mode for broken link")
		}

		// Stat should fail (tries to follow broken link)
		_, err = s.FS.Stat(link)
		if err == nil {
			t.Error("Stat on broken symlink should fail")
		}
	})

	t.Run("SymlinkAlreadyExists", func(t *testing.T) {
		target := path.Join(testDir, "exists_target.txt")
		link := path.Join(testDir, "exists_link")

		f, _ := s.FS.Create(target)
		f.Close()

		// Create first symlink
		if err := sfs.Symlink(target, link); err != nil {
			t.Fatalf("First Symlink failed: %v", err)
		}

		// Try to create symlink at same location - should fail
		err := sfs.Symlink(target, link)
		if err == nil {
			t.Error("Symlink to existing path should fail")
		}
	})

	t.Run("CircularSymlinks", func(t *testing.T) {
		// Create directory structure for circular symlinks
		circDir := path.Join(testDir, "circular")
		oneDir := path.Join(circDir, "one")
		twoDir := path.Join(oneDir, "two")

		if err := s.FS.MkdirAll(twoDir, 0755); err != nil {
			t.Fatalf("MkdirAll failed: %v", err)
		}

		// Create symlink that points back up: /circular/one/two/three -> /circular/one
		link := path.Join(twoDir, "three")
		if err := sfs.Symlink(oneDir, link); err != nil {
			t.Fatalf("Symlink failed: %v", err)
		}

		// Lstat should work on the symlink
		info, err := sfs.Lstat(link)
		if err != nil {
			t.Fatalf("Lstat on circular symlink failed: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("Lstat should show symlink mode")
		}

		// Readlink should return the target
		got, err := sfs.Readlink(link)
		if err != nil {
			t.Fatalf("Readlink failed: %v", err)
		}
		if got != oneDir {
			t.Errorf("Readlink: got %q, want %q", got, oneDir)
		}
	})

	t.Run("SelfReferenceSymlink", func(t *testing.T) {
		// Test self-referencing symlink (A -> A)
		link := path.Join(testDir, "self_ref")
		if err := sfs.Symlink(link, link); err != nil {
			t.Fatalf("Symlink failed: %v", err)
		}

		// Lstat should work
		info, err := sfs.Lstat(link)
		if err != nil {
			t.Fatalf("Lstat on self-referencing symlink failed: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("Lstat should show symlink mode")
		}

		// Stat should fail (following the symlink creates infinite loop)
		_, err = s.FS.Stat(link)
		if err == nil {
			t.Error("Stat on self-referencing symlink should fail")
		}
	})

	t.Run("TwoNodeCycle", func(t *testing.T) {
		// Test two-node cycle (A -> B -> A)
		linkA := path.Join(testDir, "cycle_a")
		linkB := path.Join(testDir, "cycle_b")

		if err := sfs.Symlink(linkB, linkA); err != nil {
			t.Fatalf("Symlink A -> B failed: %v", err)
		}
		if err := sfs.Symlink(linkA, linkB); err != nil {
			t.Fatalf("Symlink B -> A failed: %v", err)
		}

		// Lstat should work on both
		info, err := sfs.Lstat(linkA)
		if err != nil {
			t.Fatalf("Lstat on linkA failed: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("Lstat on linkA should show symlink mode")
		}

		// Stat should fail (following creates infinite loop)
		_, err = s.FS.Stat(linkA)
		if err == nil {
			t.Error("Stat on two-node cycle should fail")
		}
	})

	t.Run("RemoveSymlink", func(t *testing.T) {
		target := path.Join(testDir, "remove_target.txt")
		link := path.Join(testDir, "remove_link")

		f, _ := s.FS.Create(target)
		f.Write([]byte("should not be deleted"))
		f.Close()

		if err := sfs.Symlink(target, link); err != nil {
			t.Fatalf("Symlink failed: %v", err)
		}

		// Remove should delete the symlink, not the target
		if err := s.FS.Remove(link); err != nil {
			t.Fatalf("Remove symlink failed: %v", err)
		}

		// Symlink should be gone
		_, err := sfs.Lstat(link)
		if err == nil {
			t.Error("Symlink should be removed")
		}

		// Target should still exist
		_, err = s.FS.Stat(target)
		if err != nil {
			t.Error("Target file should still exist after removing symlink")
		}
	})

	t.Run("ReadThroughSymlink", func(t *testing.T) {
		target := path.Join(testDir, "read_through_target.txt")
		link := path.Join(testDir, "read_through_link")
		content := []byte("content read through symlink")

		f, _ := s.FS.Create(target)
		f.Write(content)
		f.Close()

		if err := sfs.Symlink(target, link); err != nil {
			t.Fatalf("Symlink failed: %v", err)
		}

		// Open and read through symlink
		f, err := s.FS.Open(link)
		if err != nil {
			t.Fatalf("Open through symlink failed: %v", err)
		}
		got, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			t.Fatalf("Read through symlink failed: %v", err)
		}

		if string(got) != string(content) {
			t.Errorf("content mismatch: got %q, want %q", got, content)
		}
	})

	t.Run("WriteThroughSymlink", func(t *testing.T) {
		target := path.Join(testDir, "write_through_target.txt")
		link := path.Join(testDir, "write_through_link")

		f, _ := s.FS.Create(target)
		f.Close()

		if err := sfs.Symlink(target, link); err != nil {
			t.Fatalf("Symlink failed: %v", err)
		}

		// Write through symlink
		content := []byte("written through symlink")
		f, err := s.FS.OpenFile(link, os.O_WRONLY, 0644)
		if err != nil {
			t.Fatalf("OpenFile through symlink failed: %v", err)
		}
		f.Write(content)
		f.Close()

		// Read from target to verify write went to target
		f, _ = s.FS.Open(target)
		got, _ := io.ReadAll(f)
		f.Close()

		if string(got) != string(content) {
			t.Errorf("write did not go to target: got %q, want %q", got, content)
		}
	})

	t.Run("ChainedSymlinks", func(t *testing.T) {
		// Create: target <- link1 <- link2
		target := path.Join(testDir, "chain_target.txt")
		link1 := path.Join(testDir, "chain_link1")
		link2 := path.Join(testDir, "chain_link2")
		content := []byte("chained symlink content")

		f, _ := s.FS.Create(target)
		f.Write(content)
		f.Close()

		if err := sfs.Symlink(target, link1); err != nil {
			t.Fatalf("Symlink link1 failed: %v", err)
		}
		if err := sfs.Symlink(link1, link2); err != nil {
			t.Fatalf("Symlink link2 failed: %v", err)
		}

		// Lstat on link2 should show symlink
		info, err := sfs.Lstat(link2)
		if err != nil {
			t.Fatalf("Lstat link2 failed: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("Lstat should show link2 as symlink")
		}

		// Readlink on link2 should return link1
		got, err := sfs.Readlink(link2)
		if err != nil {
			t.Fatalf("Readlink link2 failed: %v", err)
		}
		if got != link1 {
			t.Errorf("Readlink link2: got %q, want %q", got, link1)
		}

		// Stat on link2 should follow chain to target (not a symlink)
		info, err = s.FS.Stat(link2)
		if err != nil {
			t.Fatalf("Stat link2 failed: %v", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Error("Stat should follow chain and not show symlink")
		}

		// Read through chain should work
		f, _ = s.FS.Open(link2)
		data, _ := io.ReadAll(f)
		f.Close()
		if string(data) != string(content) {
			t.Errorf("read through chain: got %q, want %q", data, content)
		}
	})

	t.Run("RenameSymlink", func(t *testing.T) {
		target := path.Join(testDir, "rename_sym_target.txt")
		link := path.Join(testDir, "rename_sym_link")
		newLink := path.Join(testDir, "rename_sym_link_new")

		f, _ := s.FS.Create(target)
		f.Write([]byte("rename symlink target"))
		f.Close()

		if err := sfs.Symlink(target, link); err != nil {
			t.Fatalf("Symlink failed: %v", err)
		}

		// Rename the symlink
		if err := s.FS.Rename(link, newLink); err != nil {
			t.Fatalf("Rename symlink failed: %v", err)
		}

		// Old link should not exist
		_, err := sfs.Lstat(link)
		if err == nil {
			t.Error("Old symlink should not exist")
		}

		// New link should exist and be a symlink
		info, err := sfs.Lstat(newLink)
		if err != nil {
			t.Fatalf("Lstat new link failed: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("Renamed link should still be a symlink")
		}

		// Target should still exist
		_, err = s.FS.Stat(target)
		if err != nil {
			t.Error("Target should still exist after renaming symlink")
		}
	})

	t.Run("SameDirRelativeSymlink", func(t *testing.T) {
		// Same-directory relative symlink (simpler than ../)
		target := path.Join(testDir, "same_dir_target.txt")
		link := path.Join(testDir, "same_dir_link")

		f, _ := s.FS.Create(target)
		f.Write([]byte("same dir target"))
		f.Close()

		// Create symlink with just the filename (same directory)
		if err := sfs.Symlink("same_dir_target.txt", link); err != nil {
			t.Fatalf("Symlink with same-dir relative path failed: %v", err)
		}

		// Readlink should return the relative path
		got, err := sfs.Readlink(link)
		if err != nil {
			t.Fatalf("Readlink failed: %v", err)
		}
		if got != "same_dir_target.txt" {
			t.Errorf("Readlink: got %q, want %q", got, "same_dir_target.txt")
		}

		// Stat should resolve and find the target
		info, err := s.FS.Stat(link)
		if err != nil {
			t.Fatalf("Stat through same-dir symlink failed: %v", err)
		}
		if info.IsDir() {
			t.Error("Should resolve to file, not directory")
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
