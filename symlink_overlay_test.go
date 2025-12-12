package fstesting

import (
	"os"
	"syscall"
	"testing"

	"github.com/absfs/absfs"
	"github.com/absfs/memfs"
)

// Note: SymlinkOverlay is designed for filesystems WITHOUT native symlink support.
// Testing it directly with memfs doesn't work well because memfs interprets
// ModeSymlink specially in its Stat method. The proper way to test SymlinkOverlay
// would be with a filesystem that stores mode bits but doesn't interpret them.
//
// For now, we test ExtendSymlinkFiler which is the primary API and correctly
// routes to native symlinks when available.

func TestExtendSymlinkFiler_Basic(t *testing.T) {
	// memfs has native symlinks, so ExtendSymlinkFiler should use them directly
	mfs, err := memfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	sfs := absfs.ExtendSymlinkFiler(mfs)

	// Create a target file
	f, err := sfs.Create("/target.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("hello world"))
	f.Close()

	// Create a symlink
	err = sfs.Symlink("/target.txt", "/link")
	if err != nil {
		t.Fatalf("Symlink failed: %v", err)
	}

	// Read through the symlink
	content, err := sfs.ReadFile("/link")
	if err != nil {
		t.Fatalf("ReadFile through symlink failed: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("got %q, want %q", string(content), "hello world")
	}

	// Readlink should return target
	target, err := sfs.Readlink("/link")
	if err != nil {
		t.Fatalf("Readlink failed: %v", err)
	}
	if target != "/target.txt" {
		t.Errorf("got %q, want %q", target, "/target.txt")
	}

	// Lstat should show symlink mode
	info, err := sfs.Lstat("/link")
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("Lstat should show symlink mode")
	}

	// Stat should follow symlink (return target info)
	info, err = sfs.Stat("/link")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size() != 11 {
		t.Errorf("Stat should return target size, got %d", info.Size())
	}
}

func TestExtendSymlinkFiler_RelativeSymlink(t *testing.T) {
	mfs, _ := memfs.NewFS()
	sfs := absfs.ExtendSymlinkFiler(mfs)

	// Create directory structure
	sfs.MkdirAll("/a/b", 0755)

	// Create target
	f, _ := sfs.Create("/a/target.txt")
	f.Write([]byte("content"))
	f.Close()

	// Create relative symlink: /a/b/link -> ../target.txt
	err := sfs.Symlink("../target.txt", "/a/b/link")
	if err != nil {
		t.Fatalf("Symlink failed: %v", err)
	}

	// Read through relative symlink
	content, err := sfs.ReadFile("/a/b/link")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "content" {
		t.Errorf("got %q, want %q", string(content), "content")
	}
}

func TestExtendSymlinkFiler_BrokenSymlink(t *testing.T) {
	mfs, _ := memfs.NewFS()
	sfs := absfs.ExtendSymlinkFiler(mfs)

	// Create symlink to non-existent target
	err := sfs.Symlink("/nonexistent", "/broken_link")
	if err != nil {
		t.Fatalf("Symlink to nonexistent target should succeed: %v", err)
	}

	// Lstat should work
	info, err := sfs.Lstat("/broken_link")
	if err != nil {
		t.Fatalf("Lstat on broken symlink failed: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("should be a symlink")
	}

	// Stat should fail (target doesn't exist)
	_, err = sfs.Stat("/broken_link")
	if err == nil {
		t.Error("Stat on broken symlink should fail")
	}
}

func TestExtendSymlinkFiler_CircularSymlink(t *testing.T) {
	mfs, _ := memfs.NewFS()
	sfs := absfs.ExtendSymlinkFiler(mfs)

	// Create circular symlinks: a -> b, b -> a
	sfs.Symlink("/b", "/a")
	sfs.Symlink("/a", "/b")

	// Stat should fail with ELOOP
	_, err := sfs.Stat("/a")
	if err == nil {
		t.Error("Stat on circular symlink should fail")
	}
}

func TestExtendSymlinkFiler_SymlinkExists(t *testing.T) {
	mfs, _ := memfs.NewFS()
	sfs := absfs.ExtendSymlinkFiler(mfs)

	// Create a file
	f, _ := sfs.Create("/existing")
	f.Close()

	// Try to create symlink where file exists
	err := sfs.Symlink("/target", "/existing")
	if err == nil {
		t.Error("Symlink over existing file should fail")
	}
}

func TestExtendSymlinkFiler_RemoveSymlink(t *testing.T) {
	mfs, _ := memfs.NewFS()
	sfs := absfs.ExtendSymlinkFiler(mfs)

	// Create target and symlink
	f, _ := sfs.Create("/target.txt")
	f.Write([]byte("keep me"))
	f.Close()

	sfs.Symlink("/target.txt", "/link")

	// Remove symlink
	err := sfs.Remove("/link")
	if err != nil {
		t.Fatalf("Remove symlink failed: %v", err)
	}

	// Target should still exist
	content, err := sfs.ReadFile("/target.txt")
	if err != nil {
		t.Fatalf("Target was removed: %v", err)
	}
	if string(content) != "keep me" {
		t.Error("Target content changed")
	}
}

func TestExtendSymlinkFiler_Idempotent(t *testing.T) {
	mfs, _ := memfs.NewFS()

	// Call multiple times - should not double-wrap
	sfs1 := absfs.ExtendSymlinkFiler(mfs)
	sfs2 := absfs.ExtendSymlinkFiler(sfs1)

	// sfs2 should be sfs1 (returned unchanged because it's already SymlinkFileSystem)
	if _, ok := sfs1.(absfs.SymlinkFileSystem); !ok {
		t.Error("sfs1 should be SymlinkFileSystem")
	}
	if _, ok := sfs2.(absfs.SymlinkFileSystem); !ok {
		t.Error("sfs2 should be SymlinkFileSystem")
	}

	// Both should work
	f, _ := sfs2.Create("/test.txt")
	f.Close()

	err := sfs2.Symlink("/test.txt", "/link")
	if err != nil {
		t.Fatalf("Symlink failed: %v", err)
	}
}

func TestBlockSymlinks(t *testing.T) {
	mfs, _ := memfs.NewFS()
	sfs := absfs.BlockSymlinks(mfs)

	// Should be able to create files
	f, err := sfs.Create("/file.txt")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	f.Close()

	// Symlink should fail
	err = sfs.Symlink("/file.txt", "/link")
	if err == nil {
		t.Error("Symlink should fail with BlockSymlinks")
	}
	if pathErr, ok := err.(*os.PathError); ok {
		if pathErr.Err != syscall.ENOTSUP {
			t.Errorf("expected ENOTSUP, got %v", pathErr.Err)
		}
	}

	// Readlink should fail
	_, err = sfs.Readlink("/file.txt")
	if err == nil {
		t.Error("Readlink should fail with BlockSymlinks")
	}
}

// TestExtendSymlinkFilerWithFstestingSuite runs the full fstesting suite
// against ExtendSymlinkFiler to verify it behaves correctly.
func TestExtendSymlinkFilerWithFstestingSuite(t *testing.T) {
	mfs, err := memfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	sfs := absfs.ExtendSymlinkFiler(mfs)

	suite := &Suite{
		FS: sfs,
		Features: Features{
			Symlinks:      true,
			HardLinks:     false,
			Permissions:   true,
			Timestamps:    true,
			CaseSensitive: true,
			AtomicRename:  true,
			SparseFiles:   false,
			LargeFiles:    true,
		},
	}

	suite.Run(t)
}
