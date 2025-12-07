package fstesting

import (
	"bytes"
	"io"
	"os"
	"path"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/absfs/absfs"
)

// FuzzCreate tests file creation with arbitrary paths.
// It verifies that:
// - Valid paths can be created without panics
// - Created files can be stat'd and read back
// - Invalid paths fail gracefully (no panics)
func FuzzCreate(f *testing.F, fs absfs.FileSystem, testDir string) {
	// Seed corpus with interesting cases
	f.Add("test.txt")
	f.Add("nested/path/file.txt")
	f.Add(".hidden")
	f.Add("spaces in name.txt")
	f.Add("file.multiple.dots.txt")
	f.Add("UPPERCASE.TXT")
	f.Add("mixedCase.Txt")
	f.Add("日本語.txt")
	f.Add("émoji🎉.txt")
	f.Add(strings.Repeat("a", 255)) // Max filename length on most systems

	f.Fuzz(func(t *testing.T, name string) {
		// Skip obviously invalid names
		if name == "" || name == "." || name == ".." {
			return
		}
		if strings.ContainsAny(name, "\x00") {
			return // Null bytes are never valid in paths
		}
		if !utf8.ValidString(name) {
			return // Skip invalid UTF-8
		}

		filePath := path.Join(testDir, "fuzz_create", name)

		// Ensure parent directory exists
		dir := path.Dir(filePath)
		if err := fs.MkdirAll(dir, 0755); err != nil {
			return // Parent creation failed, that's ok
		}

		// Attempt to create file - should not panic
		file, err := fs.Create(filePath)
		if err != nil {
			return // Creation failed, that's ok
		}
		defer file.Close()

		// If creation succeeded, verify we can stat it
		info, err := fs.Stat(filePath)
		if err != nil {
			t.Errorf("created file but Stat failed: %v", err)
			return
		}

		if info.IsDir() {
			t.Error("created file but IsDir() returned true")
		}

		// Clean up
		fs.Remove(filePath)
	})
}

// FuzzReadWrite tests read/write operations with arbitrary data.
// It verifies that:
// - Data survives a write/read roundtrip
// - No panics occur with any data
func FuzzReadWrite(f *testing.F, fs absfs.FileSystem, testDir string) {
	// Seed corpus
	f.Add([]byte("hello"))
	f.Add([]byte(""))
	f.Add([]byte{0x00})
	f.Add([]byte{0xFF})
	f.Add([]byte{0x00, 0xFF, 0x00, 0xFF})
	f.Add(make([]byte, 4096))        // Page-sized
	f.Add(make([]byte, 4097))        // Just over page size
	f.Add([]byte("日本語テスト"))          // UTF-8 text
	f.Add(bytes.Repeat([]byte("x"), 1<<16)) // 64KB

	counter := 0
	f.Fuzz(func(t *testing.T, data []byte) {
		counter++
		filePath := path.Join(testDir, "fuzz_rw", string(rune('a'+counter%26))+".bin")

		// Ensure directory exists
		fs.MkdirAll(path.Dir(filePath), 0755)

		// Write
		file, err := fs.Create(filePath)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		n, err := file.Write(data)
		if err != nil {
			file.Close()
			fs.Remove(filePath)
			return // Write error is ok for fuzz testing
		}
		if n != len(data) {
			t.Errorf("Write returned %d, want %d", n, len(data))
		}
		file.Close()

		// Read back
		file, err = fs.Open(filePath)
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}

		got, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}

		if !bytes.Equal(got, data) {
			t.Errorf("data mismatch: wrote %d bytes, read %d bytes", len(data), len(got))
		}

		// Clean up
		fs.Remove(filePath)
	})
}

// FuzzRename tests rename operations with arbitrary paths.
func FuzzRename(f *testing.F, fs absfs.FileSystem, testDir string) {
	f.Add("old.txt", "new.txt")
	f.Add("a", "b")
	f.Add("file.txt", "subdir/file.txt")
	f.Add("日本語.txt", "renamed.txt")
	f.Add("source", "target with spaces")

	f.Fuzz(func(t *testing.T, oldName, newName string) {
		// Skip invalid names
		if oldName == "" || newName == "" {
			return
		}
		if strings.ContainsAny(oldName+newName, "\x00") {
			return
		}
		if oldName == "." || oldName == ".." || newName == "." || newName == ".." {
			return
		}

		oldPath := path.Join(testDir, "fuzz_rename", oldName)
		newPath := path.Join(testDir, "fuzz_rename", newName)

		// Create source file
		fs.MkdirAll(path.Dir(oldPath), 0755)
		file, err := fs.Create(oldPath)
		if err != nil {
			return
		}
		content := []byte("rename test content")
		file.Write(content)
		file.Close()

		// Ensure target directory exists
		fs.MkdirAll(path.Dir(newPath), 0755)

		// Attempt rename
		err = fs.Rename(oldPath, newPath)
		if err != nil {
			fs.Remove(oldPath) // Clean up source
			return
		}

		// Verify old path gone
		if _, err := fs.Stat(oldPath); !os.IsNotExist(err) {
			t.Error("old path should not exist after rename")
		}

		// Verify new path exists with content
		file, err = fs.Open(newPath)
		if err != nil {
			t.Errorf("cannot open renamed file: %v", err)
		} else {
			got, _ := io.ReadAll(file)
			file.Close()
			if !bytes.Equal(got, content) {
				t.Error("content mismatch after rename")
			}
		}

		// Clean up
		fs.Remove(newPath)
	})
}

// FuzzMkdir tests directory creation with arbitrary paths.
func FuzzMkdir(f *testing.F, fs absfs.FileSystem, testDir string) {
	f.Add("simple")
	f.Add("nested/path")
	f.Add("deeply/nested/directory/path")
	f.Add("with spaces")
	f.Add("日本語ディレクトリ")
	f.Add(strings.Repeat("long", 50))

	f.Fuzz(func(t *testing.T, name string) {
		if name == "" || strings.ContainsAny(name, "\x00") {
			return
		}
		if name == "." || name == ".." {
			return
		}

		dirPath := path.Join(testDir, "fuzz_mkdir", name)

		err := fs.MkdirAll(dirPath, 0755)
		if err != nil {
			return // Mkdir can fail for various valid reasons
		}

		// Verify it's a directory
		info, err := fs.Stat(dirPath)
		if err != nil {
			t.Errorf("MkdirAll succeeded but Stat failed: %v", err)
			return
		}

		if !info.IsDir() {
			t.Error("created path is not a directory")
		}

		// Clean up
		fs.RemoveAll(dirPath)
	})
}

// FuzzPathTraversal tests that path traversal attacks are handled safely.
// This is particularly important for basefs and similar sandboxing wrappers.
func FuzzPathTraversal(f *testing.F, fs absfs.FileSystem, testDir string) {
	f.Add("../escape")
	f.Add("../../etc/passwd")
	f.Add("subdir/../../../escape")
	f.Add("....//....//escape")
	f.Add("..\\..\\escape")
	f.Add("subdir/./../../escape")
	f.Add(strings.Repeat("../", 100) + "escape")

	f.Fuzz(func(t *testing.T, testPath string) {
		if testPath == "" || strings.ContainsAny(testPath, "\x00") {
			return
		}

		fullPath := path.Join(testDir, testPath)

		// These operations should either:
		// 1. Fail with an error (preferred for sandboxed fs)
		// 2. Succeed but stay within testDir
		// 3. NOT panic

		// Try various operations
		_, _ = fs.Stat(fullPath)
		_, _ = fs.Open(fullPath)
		_ = fs.Mkdir(fullPath, 0755)

		// The key invariant: no panics occurred
	})
}

// FuzzOpenFlags tests opening files with various flag combinations.
func FuzzOpenFlags(f *testing.F, fs absfs.FileSystem, testDir string) {
	// Common flag combinations
	f.Add(os.O_RDONLY)
	f.Add(os.O_WRONLY)
	f.Add(os.O_RDWR)
	f.Add(os.O_CREATE)
	f.Add(os.O_CREATE | os.O_EXCL)
	f.Add(os.O_CREATE | os.O_TRUNC)
	f.Add(os.O_APPEND)
	f.Add(os.O_APPEND | os.O_WRONLY)
	f.Add(os.O_SYNC)
	f.Add(os.O_CREATE | os.O_RDWR | os.O_TRUNC)

	counter := 0
	f.Fuzz(func(t *testing.T, flags int) {
		counter++
		filePath := path.Join(testDir, "fuzz_flags", string(rune('a'+counter%26))+".txt")
		fs.MkdirAll(path.Dir(filePath), 0755)

		// Pre-create file for some tests
		if counter%2 == 0 {
			if f, err := fs.Create(filePath); err == nil {
				f.Write([]byte("existing content"))
				f.Close()
			}
		}

		// Try to open with given flags - should not panic
		file, err := fs.OpenFile(filePath, flags, 0644)
		if err != nil {
			return // Open errors are expected for invalid flag combos
		}
		defer file.Close()

		// If we opened successfully, try basic operations
		if flags&(os.O_WRONLY|os.O_RDWR) != 0 {
			file.Write([]byte("test"))
		}
		if flags&os.O_WRONLY == 0 {
			buf := make([]byte, 10)
			file.Read(buf)
		}

		// Clean up
		fs.Remove(filePath)
	})
}
