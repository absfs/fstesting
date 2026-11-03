package fstesting

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
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

	// Skips lists subtests to skip, each with the reason it is skipped.
	// Run and RunWithSkips both honor it. See Skip for the pattern rules.
	Skips []Skip
}

// Run executes all applicable tests based on the configured features,
// skipping the subtests selected by s.Skips.
func (s *Suite) Run(t *testing.T) {
	t.Helper()
	s.run(realT{t}, s.Skips)
}

// RunWithSkips is Run with additional subtests to skip, for implementations
// with known limitations. Each pattern follows the rules documented on Skip:
// matching is case-insensitive, "Symlinks/Lstat" names exactly one subtest,
// "Lstat" matches that name at any depth, and "Symlinks" skips the whole
// group. The patterns are added to s.Skips, and every subtest they select is
// skipped with a t.Skip message naming the subtest and the pattern. To record
// why each subtest is skipped, set s.Skips instead.
//
// An empty or malformed pattern fails the test before any subtest runs.
func (s *Suite) RunWithSkips(t *testing.T, skips ...string) {
	t.Helper()
	all := append([]Skip(nil), s.Skips...)
	for _, p := range skips {
		all = append(all, Skip{Pattern: p, Reason: runWithSkipsReason})
	}
	s.run(realT{t}, all)
}

// run is the body of Run and RunWithSkips.
func (s *Suite) run(root tb, skips []Skip) {
	root.Helper()

	rules, err := compileSkips(skips)
	if err != nil {
		root.Fatal(err)
	}
	t := skipT{tb: root, rules: rules}
	defer logUnmatched(root, rules)

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
	t.Run("FileOperations", func(t tb) {
		s.testFileOperations(t, testDir)
	})

	t.Run("DirectoryOperations", func(t tb) {
		s.testDirectoryOperations(t, testDir)
	})

	t.Run("PathHandling", func(t tb) {
		s.testPathHandling(t, testDir)
	})

	t.Run("ErrorSemantics", func(t tb) {
		s.testErrorSemantics(t, testDir)
	})

	t.Run("NewFilerMethods", func(t tb) {
		s.testNewFilerMethods(t, testDir)
	})

	if s.Features.Symlinks {
		t.Run("Symlinks", func(t tb) {
			s.testSymlinks(t, testDir)
		})
	}

	if s.Features.Permissions {
		t.Run("Permissions", func(t tb) {
			s.testPermissions(t, testDir)
		})
	}

	if s.Features.Timestamps {
		t.Run("Timestamps", func(t tb) {
			s.testTimestamps(t, testDir)
		})
	}
}

// testFileOperations tests basic file CRUD operations.
func (s *Suite) testFileOperations(t tb, testDir string) {
	t.Helper()

	t.Run("CreateAndRead", func(t tb) {
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

	t.Run("OpenFile", func(t tb) {
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

	t.Run("Truncate", func(t tb) {
		filePath := path.Join(testDir, "truncate_test.txt")
		content := []byte("hello, world")

		// Create file with content
		s.writeFile(t, filePath, content)

		// Truncate to 5 bytes
		if err := s.FS.Truncate(filePath, 5); err != nil {
			t.Fatalf("Truncate failed: %v", err)
		}

		// Verify size
		info := s.stat(t, filePath)
		if info.Size() != 5 {
			t.Errorf("size after truncate: got %d, want 5", info.Size())
		}
	})

	t.Run("Remove", func(t tb) {
		filePath := path.Join(testDir, "remove_test.txt")

		// Create and remove
		s.writeFile(t, filePath, nil)

		if err := s.FS.Remove(filePath); err != nil {
			t.Fatalf("Remove failed: %v", err)
		}

		// Verify gone
		_, err := s.FS.Stat(filePath)
		if !os.IsNotExist(err) {
			t.Errorf("file should not exist after Remove, got err: %v", err)
		}
	})

	t.Run("Rename", func(t tb) {
		oldPath := path.Join(testDir, "rename_old.txt")
		newPath := path.Join(testDir, "rename_new.txt")
		content := []byte("rename test content")

		// Create file
		s.writeFile(t, oldPath, content)

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
		got := s.readFile(t, newPath)
		if !bytes.Equal(got, content) {
			t.Errorf("content mismatch after rename: got %q, want %q", got, content)
		}
	})

	t.Run("Stat", func(t tb) {
		filePath := path.Join(testDir, "stat_test.txt")
		content := []byte("stat test")

		s.writeFile(t, filePath, content)

		info := s.stat(t, filePath)

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
func (s *Suite) testDirectoryOperations(t tb, testDir string) {
	t.Helper()

	t.Run("Mkdir", func(t tb) {
		dirPath := path.Join(testDir, "mkdir_test")

		if err := s.FS.Mkdir(dirPath, 0755); err != nil {
			t.Fatalf("Mkdir failed: %v", err)
		}

		info := s.stat(t, dirPath)

		if !info.IsDir() {
			t.Error("created path should be a directory")
		}
	})

	t.Run("MkdirAll", func(t tb) {
		dirPath := path.Join(testDir, "mkdirall", "nested", "path")

		if err := s.FS.MkdirAll(dirPath, 0755); err != nil {
			t.Fatalf("MkdirAll failed: %v", err)
		}

		info := s.stat(t, dirPath)

		if !info.IsDir() {
			t.Error("created path should be a directory")
		}
	})

	t.Run("RemoveAll", func(t tb) {
		base := path.Join(testDir, "removeall_test")
		nested := path.Join(base, "nested")
		file := path.Join(nested, "file.txt")

		s.FS.MkdirAll(nested, 0755)
		s.writeFile(t, file, nil)

		if err := s.FS.RemoveAll(base); err != nil {
			t.Fatalf("RemoveAll failed: %v", err)
		}

		_, err := s.FS.Stat(base)
		if !os.IsNotExist(err) {
			t.Error("directory should not exist after RemoveAll")
		}
	})

	t.Run("ReadDir", func(t tb) {
		base := path.Join(testDir, "readdir_test")
		s.FS.Mkdir(base, 0755)

		// Create some files
		names := []string{"a.txt", "b.txt", "c.txt"}
		for _, name := range names {
			s.writeFile(t, path.Join(base, name), nil)
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
func (s *Suite) testPathHandling(t tb, testDir string) {
	t.Helper()

	t.Run("DotPaths", func(t tb) {
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

	t.Run("TrailingSlash", func(t tb) {
		base := path.Join(testDir, "trailingslash")
		s.FS.Mkdir(base, 0755)

		// Stat directory with trailing slash
		_, err := s.FS.Stat(base + "/")
		if err != nil {
			t.Errorf("Stat with trailing slash failed: %v", err)
		}
	})

	t.Run("SpecialCharacters", func(t tb) {
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

	t.Run("Unicode", func(t tb) {
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
func (s *Suite) testErrorSemantics(t tb, testDir string) {
	t.Helper()

	t.Run("NotExist", func(t tb) {
		_, err := s.FS.Stat(path.Join(testDir, "nonexistent"))
		if !os.IsNotExist(err) {
			t.Errorf("Stat nonexistent: expected os.IsNotExist, got %v", err)
		}

		_, err = s.FS.Open(path.Join(testDir, "nonexistent"))
		if !os.IsNotExist(err) {
			t.Errorf("Open nonexistent: expected os.IsNotExist, got %v", err)
		}
	})

	t.Run("Exist", func(t tb) {
		dirPath := path.Join(testDir, "exist_test")
		s.FS.Mkdir(dirPath, 0755)

		err := s.FS.Mkdir(dirPath, 0755)
		if !os.IsExist(err) {
			t.Errorf("Mkdir existing: expected os.IsExist, got %v", err)
		}
	})

	t.Run("IsDir", func(t tb) {
		dirPath := path.Join(testDir, "isdir_test")
		s.FS.Mkdir(dirPath, 0755)

		// Try to open directory as file for writing
		_, err := s.FS.OpenFile(dirPath, os.O_WRONLY, 0644)
		if err == nil {
			t.Error("OpenFile directory for writing should fail")
		}
	})

	t.Run("NotDir", func(t tb) {
		filePath := path.Join(testDir, "notdir_test.txt")
		s.writeFile(t, filePath, nil)

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
func (s *Suite) testNewFilerMethods(t tb, testDir string) {
	t.Helper()

	t.Run("Filer.ReadDir", func(t tb) {
		base := path.Join(testDir, "filer_readdir_test")
		s.FS.MkdirAll(base, 0755)

		// Create some files
		names := []string{"file1.txt", "file2.txt", "file3.txt"}
		for _, name := range names {
			s.writeFile(t, path.Join(base, name), nil)
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

	t.Run("Filer.ReadFile", func(t tb) {
		filePath := path.Join(testDir, "readfile_test.txt")
		content := []byte("test content for ReadFile")

		// Create file
		s.writeFile(t, filePath, content)

		// Test Filer.ReadFile
		got, err := s.FS.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Filer.ReadFile failed: %v", err)
		}

		if !bytes.Equal(got, content) {
			t.Errorf("content mismatch: got %q, want %q", got, content)
		}
	})

	t.Run("Filer.Sub", func(t tb) {
		// Create a directory structure
		base := path.Join(testDir, "sub_test")
		nested := path.Join(base, "nested")
		s.FS.MkdirAll(nested, 0755)

		// Create a file in the nested directory
		filePath := path.Join(nested, "file.txt")
		content := []byte("nested file content")
		s.writeFile(t, filePath, content)

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
		s.writeFile(t, filePath2, nil)

		_, err = s.FS.Sub(filePath2)
		if err == nil {
			t.Error("Sub on a file should return an error")
		}
	})

	t.Run("Filer.Sub.Nested", func(t tb) {
		// Test nested Sub calls
		base := path.Join(testDir, "nested_sub_test")
		level1 := path.Join(base, "level1")
		level2 := path.Join(level1, "level2")
		s.FS.MkdirAll(level2, 0755)

		// Create a file at level2
		filePath := path.Join(level2, "deep.txt")
		content := []byte("deep nested content")
		s.writeFile(t, filePath, content)

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
func (s *Suite) testSymlinks(t tb, testDir string) {
	t.Helper()

	// Check if FS implements SymlinkFileSystem
	sfs, ok := s.FS.(absfs.SymlinkFileSystem)
	if !ok {
		t.Skip("filesystem does not implement SymlinkFileSystem")
	}

	t.Run("CreateAndRead", func(t tb) {
		target := path.Join(testDir, "symlink_target.txt")
		link := path.Join(testDir, "symlink_link")

		// Create target
		s.writeFile(t, target, []byte("symlink target content"))

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

	t.Run("Lstat", func(t tb) {
		target := path.Join(testDir, "lstat_target.txt")
		link := path.Join(testDir, "lstat_link")

		s.writeFile(t, target, nil)
		if err := sfs.Symlink(target, link); err != nil {
			t.Fatalf("Symlink failed: %v", err)
		}

		// Stat follows symlink. A Stat error here means the link did not
		// resolve to its target (for example, a routing layer rewrote it);
		// stat reports that as a failure rather than returning nil info.
		info := s.stat(t, link)
		if info.Mode()&os.ModeSymlink != 0 {
			t.Error("Stat should follow symlink")
		}

		// Lstat does not follow symlink
		info = lstat(t, sfs, link)
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("Lstat should return symlink info")
		}
	})

	t.Run("RelativeSymlink", func(t tb) {
		// Create directory structure for relative symlink
		subdir := path.Join(testDir, "rel_sub")
		if err := s.FS.MkdirAll(subdir, 0755); err != nil {
			t.Fatalf("MkdirAll failed: %v", err)
		}

		target := path.Join(testDir, "rel_target.txt")
		link := path.Join(subdir, "rel_link")

		// Create target file
		s.writeFile(t, target, []byte("relative target"))

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

	t.Run("SymlinkToDirectory", func(t tb) {
		targetDir := path.Join(testDir, "link_target_dir")
		link := path.Join(testDir, "dir_link")

		if err := s.FS.Mkdir(targetDir, 0755); err != nil {
			t.Fatalf("Mkdir failed: %v", err)
		}

		// Create a file inside the target directory
		s.writeFile(t, path.Join(targetDir, "file.txt"), []byte("content"))

		// Create symlink to directory
		if err := sfs.Symlink(targetDir, link); err != nil {
			t.Fatalf("Symlink to directory failed: %v", err)
		}

		// Stat should follow and show it as a directory
		info := s.stat(t, link)
		if !info.IsDir() {
			t.Error("Stat should show symlink-to-dir as directory")
		}

		// Lstat should show it as a symlink
		info = lstat(t, sfs, link)
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("Lstat should show symlink mode")
		}
	})

	t.Run("BrokenSymlink", func(t tb) {
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
		info := lstat(t, sfs, link)
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("Lstat should show symlink mode for broken link")
		}

		// Stat should fail (tries to follow broken link)
		_, err = s.FS.Stat(link)
		if err == nil {
			t.Error("Stat on broken symlink should fail")
		}
	})

	t.Run("SymlinkAlreadyExists", func(t tb) {
		target := path.Join(testDir, "exists_target.txt")
		link := path.Join(testDir, "exists_link")

		s.writeFile(t, target, nil)

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

	t.Run("CircularSymlinks", func(t tb) {
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
		info := lstat(t, sfs, link)
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

	t.Run("SelfReferenceSymlink", func(t tb) {
		// Test self-referencing symlink (A -> A)
		link := path.Join(testDir, "self_ref")
		if err := sfs.Symlink(link, link); err != nil {
			t.Fatalf("Symlink failed: %v", err)
		}

		// Lstat should work
		info := lstat(t, sfs, link)
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("Lstat should show symlink mode")
		}

		// Stat should fail (following the symlink creates infinite loop)
		_, err := s.FS.Stat(link)
		if err == nil {
			t.Error("Stat on self-referencing symlink should fail")
		}
	})

	t.Run("TwoNodeCycle", func(t tb) {
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
		info := lstat(t, sfs, linkA)
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("Lstat on linkA should show symlink mode")
		}

		// Stat should fail (following creates infinite loop)
		_, err := s.FS.Stat(linkA)
		if err == nil {
			t.Error("Stat on two-node cycle should fail")
		}
	})

	t.Run("RemoveSymlink", func(t tb) {
		target := path.Join(testDir, "remove_target.txt")
		link := path.Join(testDir, "remove_link")

		s.writeFile(t, target, []byte("should not be deleted"))

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

	t.Run("ReadThroughSymlink", func(t tb) {
		target := path.Join(testDir, "read_through_target.txt")
		link := path.Join(testDir, "read_through_link")
		content := []byte("content read through symlink")

		s.writeFile(t, target, content)

		if err := sfs.Symlink(target, link); err != nil {
			t.Fatalf("Symlink failed: %v", err)
		}

		// Open and read through symlink
		got := s.readFile(t, link)
		if string(got) != string(content) {
			t.Errorf("content mismatch: got %q, want %q", got, content)
		}
	})

	t.Run("WriteThroughSymlink", func(t tb) {
		target := path.Join(testDir, "write_through_target.txt")
		link := path.Join(testDir, "write_through_link")

		s.writeFile(t, target, nil)

		if err := sfs.Symlink(target, link); err != nil {
			t.Fatalf("Symlink failed: %v", err)
		}

		// Write through symlink
		content := []byte("written through symlink")
		f, err := s.FS.OpenFile(link, os.O_WRONLY, 0644)
		if err != nil {
			t.Fatalf("OpenFile through symlink failed: %v", err)
		}
		_, werr := f.Write(content)
		f.Close()
		if werr != nil {
			t.Fatalf("Write through symlink failed: %v", werr)
		}

		// Read from target to verify write went to target
		got := s.readFile(t, target)
		if string(got) != string(content) {
			t.Errorf("write did not go to target: got %q, want %q", got, content)
		}
	})

	t.Run("ChainedSymlinks", func(t tb) {
		// Create: target <- link1 <- link2
		target := path.Join(testDir, "chain_target.txt")
		link1 := path.Join(testDir, "chain_link1")
		link2 := path.Join(testDir, "chain_link2")
		content := []byte("chained symlink content")

		s.writeFile(t, target, content)

		if err := sfs.Symlink(target, link1); err != nil {
			t.Fatalf("Symlink link1 failed: %v", err)
		}
		if err := sfs.Symlink(link1, link2); err != nil {
			t.Fatalf("Symlink link2 failed: %v", err)
		}

		// Lstat on link2 should show symlink
		info := lstat(t, sfs, link2)
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
		info = s.stat(t, link2)
		if info.Mode()&os.ModeSymlink != 0 {
			t.Error("Stat should follow chain and not show symlink")
		}

		// Read through chain should work
		data := s.readFile(t, link2)
		if string(data) != string(content) {
			t.Errorf("read through chain: got %q, want %q", data, content)
		}
	})

	t.Run("RenameSymlink", func(t tb) {
		target := path.Join(testDir, "rename_sym_target.txt")
		link := path.Join(testDir, "rename_sym_link")
		newLink := path.Join(testDir, "rename_sym_link_new")

		s.writeFile(t, target, []byte("rename symlink target"))

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
		info := lstat(t, sfs, newLink)
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("Renamed link should still be a symlink")
		}

		// Target should still exist
		_, err = s.FS.Stat(target)
		if err != nil {
			t.Error("Target should still exist after renaming symlink")
		}
	})

	t.Run("SameDirRelativeSymlink", func(t tb) {
		// Same-directory relative symlink (simpler than ../)
		target := path.Join(testDir, "same_dir_target.txt")
		link := path.Join(testDir, "same_dir_link")

		s.writeFile(t, target, []byte("same dir target"))

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
		info := s.stat(t, link)
		if info.IsDir() {
			t.Error("Should resolve to file, not directory")
		}
	})
}

// testPermissions tests permission-related operations.
func (s *Suite) testPermissions(t tb, testDir string) {
	t.Helper()

	t.Run("Chmod", func(t tb) {
		filePath := path.Join(testDir, "chmod_test.txt")
		s.writeFile(t, filePath, nil)

		if err := s.FS.Chmod(filePath, 0600); err != nil {
			t.Fatalf("Chmod failed: %v", err)
		}

		info := s.stat(t, filePath)
		// Mask out non-permission bits
		got := info.Mode().Perm()
		if got != 0600 {
			t.Errorf("mode after Chmod: got %o, want 0600", got)
		}
	})
}

// testTimestamps tests timestamp-related operations.
func (s *Suite) testTimestamps(t tb, testDir string) {
	t.Helper()

	t.Run("Chtimes", func(t tb) {
		filePath := path.Join(testDir, "chtimes_test.txt")
		s.writeFile(t, filePath, nil)

		atime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		mtime := time.Date(2021, 6, 15, 12, 0, 0, 0, time.UTC)

		if err := s.FS.Chtimes(filePath, atime, mtime); err != nil {
			t.Fatalf("Chtimes failed: %v", err)
		}

		info := s.stat(t, filePath)
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
	s.quickCheck(realT{t})
}

func (s *Suite) quickCheck(t tb) {
	t.Helper()

	testDir := path.Join(s.FS.TempDir(), fmt.Sprintf("fstesting_quick_%d", time.Now().UnixNano()))
	if err := s.FS.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	defer s.FS.RemoveAll(testDir)

	// Test create/read/delete cycle
	filePath := path.Join(testDir, "quickcheck.txt")
	content := []byte("quick check content")

	s.writeFile(t, filePath, content)

	got := s.readFile(t, filePath)
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

// writeFile creates p holding content, failing the test on any error.
func (s *Suite) writeFile(t tb, p string, content []byte) {
	t.Helper()
	f, err := s.FS.Create(p)
	if err != nil {
		t.Fatalf("Create %s failed: %v", p, err)
	}
	if f == nil {
		t.Fatalf("Create %s returned a nil file and no error", p)
	}
	var werr error
	if len(content) > 0 {
		_, werr = f.Write(content)
	}
	cerr := f.Close()
	if werr != nil {
		t.Fatalf("Write %s failed: %v", p, werr)
	}
	if cerr != nil {
		t.Fatalf("Close %s failed: %v", p, cerr)
	}
}

// readFile opens p and returns its contents, failing the test on any error.
func (s *Suite) readFile(t tb, p string) []byte {
	t.Helper()
	f, err := s.FS.Open(p)
	if err != nil {
		t.Fatalf("Open %s failed: %v", p, err)
	}
	if f == nil {
		t.Fatalf("Open %s returned a nil file and no error", p)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("Read %s failed: %v", p, err)
	}
	return got
}

// stat returns s.FS.Stat(p), failing the test on an error or a nil result.
func (s *Suite) stat(t tb, p string) os.FileInfo {
	t.Helper()
	info, err := s.FS.Stat(p)
	return checkInfo(t, "Stat", p, info, err)
}

// lstat returns sfs.Lstat(p), failing the test on an error or a nil result.
func lstat(t tb, sfs absfs.SymlinkFileSystem, p string) os.FileInfo {
	t.Helper()
	info, err := sfs.Lstat(p)
	return checkInfo(t, "Lstat", p, info, err)
}

// checkInfo fails t unless a stat-like call on p returned a non-nil FileInfo
// and no error, so callers never dereference a nil result.
func checkInfo(t tb, op, p string, info os.FileInfo, err error) os.FileInfo {
	t.Helper()
	if err != nil {
		t.Fatalf("%s %s failed: %v", op, p, err)
	}
	if info == nil {
		t.Fatalf("%s %s returned nil FileInfo and no error", op, p)
	}
	return info
}
