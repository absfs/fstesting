# fstesting Guide

## Table of Contents

- [Basic Usage](#basic-usage)
- [Feature Configuration](#feature-configuration)
- [Test Categories](#test-categories)
- [Quick Check](#quick-check)
- [Wrapper Testing](#wrapper-testing)
- [Fuzz Testing](#fuzz-testing)
- [Custom Test Directory](#custom-test-directory)

## Basic Usage

The simplest way to test a filesystem:

```go
package myfs_test

import (
    "testing"
    "github.com/absfs/fstesting"
    "github.com/example/myfs"
)

func TestMyFS(t *testing.T) {
    fs, _ := myfs.New()

    suite := &fstesting.Suite{
        FS:       fs,
        Features: fstesting.DefaultFeatures(),
    }

    suite.Run(t)
}
```

This runs all applicable tests based on your feature configuration.

## Feature Configuration

The `Features` struct controls which tests run:

```go
type Features struct {
    Symlinks      bool // Symbolic link support
    HardLinks     bool // Hard link support
    Permissions   bool // chmod/file permissions
    Timestamps    bool // chtimes/modification times
    CaseSensitive bool // Case-sensitive filenames
    AtomicRename  bool // Atomic rename operations
    SparseFiles   bool // Sparse file support
    LargeFiles    bool // Files > 4GB
}
```

### DefaultFeatures

Use `DefaultFeatures()` for full OS-like filesystem support:

```go
features := fstesting.DefaultFeatures()
// All features enabled
```

### Custom Features

For filesystems with limited capabilities:

```go
// In-memory filesystem without symlinks or permissions
features := fstesting.Features{
    Symlinks:      false,
    HardLinks:     false,
    Permissions:   false,
    Timestamps:    true,
    CaseSensitive: true,
    AtomicRename:  true,
    SparseFiles:   false,
    LargeFiles:    false,
}
```

### Read-Only Filesystem

```go
features := fstesting.Features{
    // All write-related features disabled
    Symlinks:    false,
    Permissions: false,
    Timestamps:  false,
}
```

## Test Categories

The suite runs tests in these categories:

### FileOperations

Core file operations every filesystem must support:

- **CreateAndRead** - Create file, write content, read back
- **OpenFile** - Open with various flags (O_CREATE, O_TRUNC, O_APPEND, etc.)
- **Truncate** - Truncate files to specific sizes
- **Remove** - Delete files
- **Rename** - Move/rename files
- **Stat** - Get file metadata

### DirectoryOperations

Directory management:

- **Mkdir** - Create single directory
- **MkdirAll** - Create nested directory tree
- **RemoveAll** - Recursively delete directory
- **ReadDir** - List directory contents

### PathHandling

Path edge cases:

- **DotPaths** - Handling of `.` and `..`
- **TrailingSlash** - Paths ending in `/`
- **SpecialCharacters** - Spaces, dashes, underscores
- **Unicode** - Non-ASCII filenames

### ErrorSemantics

Correct error types:

- **NotExist** - `os.IsNotExist()` for missing files
- **Exist** - `os.IsExist()` for conflicts
- **IsDir** - Errors when file expected but got directory
- **NotDir** - Errors when directory expected but got file

### Symlinks (when enabled)

Symbolic link operations:

- **CreateAndRead** - Create symlink, read target
- **Lstat** - Get symlink metadata without following

### Permissions (when enabled)

- **Chmod** - Change file permissions

### Timestamps (when enabled)

- **Chtimes** - Modify access/modification times

## Quick Check

For fast iteration during development:

```go
func TestMyFS_Quick(t *testing.T) {
    fs, _ := myfs.New()

    suite := &fstesting.Suite{
        FS:       fs,
        Features: fstesting.DefaultFeatures(),
    }

    suite.QuickCheck(t) // Fast sanity test
}
```

`QuickCheck` runs a minimal set of tests to verify basic functionality:
- Create and read a file
- Create and list a directory
- Remove file and directory

## Wrapper Testing

For filesystem wrappers (compression, encryption, caching, etc.):

```go
func TestCompressWrapper(t *testing.T) {
    baseFS, _ := memfs.New()

    suite := &fstesting.WrapperSuite{
        Factory: func(base absfs.FileSystem) (absfs.FileSystem, error) {
            return compressfs.New(base)
        },
        BaseFS:         baseFS,
        Name:           "compressfs",
        TransformsData: true,  // Wrapper modifies file contents
        TransformsMeta: false, // Wrapper preserves metadata
        ReadOnly:       false,
    }

    suite.Run(t)
}
```

### WrapperSuite Fields

| Field | Description |
|-------|-------------|
| `Factory` | Function to create wrapper around base filesystem |
| `BaseFS` | The underlying filesystem to wrap |
| `Name` | Descriptive name for test output |
| `TransformsData` | True if wrapper modifies file contents |
| `TransformsMeta` | True if wrapper modifies metadata |
| `ReadOnly` | True if wrapper blocks write operations |
| `TestDir` | Custom test directory (optional) |

### Wrapper Test Categories

- **Passthrough** - Basic operations work through wrapper
- **DataIntegrity** - Data survives write/read cycle (various sizes, binary, unicode)
- **WriteBlocking** - Write operations fail (for read-only wrappers)
- **TransformRoundtrip** - Data survives transformation (for TransformsData wrappers)

## Fuzz Testing

Property-based tests for thorough validation:

```go
func FuzzMyFS_ReadWrite(f *testing.F) {
    fs, _ := myfs.New()
    testDir := filepath.Join(fs.TempDir(), "fuzz")
    fs.MkdirAll(testDir, 0755)

    fstesting.FuzzReadWrite(f, fs, testDir)
}
```

### Available Fuzz Tests

| Function | Tests |
|----------|-------|
| `FuzzCreate` | File creation with arbitrary paths |
| `FuzzReadWrite` | Data integrity through write/read cycles |
| `FuzzRename` | Rename operations with arbitrary paths |
| `FuzzMkdir` | Directory creation with arbitrary paths |
| `FuzzPathTraversal` | Path traversal attack resistance |
| `FuzzOpenFlags` | OpenFile with various flag combinations |

### Running Fuzz Tests

```bash
# Run for 30 seconds
go test -fuzz=FuzzMyFS_ReadWrite -fuzztime=30s

# Run until failure
go test -fuzz=FuzzMyFS_ReadWrite
```

### Wrapper Fuzz Testing

```go
func FuzzCompressWrapper(f *testing.F) {
    baseFS, _ := memfs.New()
    testDir := filepath.Join(baseFS.TempDir(), "fuzz")

    fstesting.FuzzWrapperRoundtrip(f,
        func(base absfs.FileSystem) (absfs.FileSystem, error) {
            return compressfs.New(base)
        },
        baseFS,
        testDir,
    )
}
```

## Custom Test Directory

By default, tests use `fs.TempDir()`. To specify a custom location:

```go
suite := &fstesting.Suite{
    FS:       fs,
    Features: fstesting.DefaultFeatures(),
    TestDir:  "/custom/test/path",
}
```

To preserve the test directory after tests (for debugging):

```go
suite := &fstesting.Suite{
    FS:          fs,
    Features:    fstesting.DefaultFeatures(),
    KeepTestDir: true,
}
```

## Node Type Categories

The package recognizes different types of absfs packages:

| Type | Description | Examples |
|------|-------------|----------|
| Core | Interface definitions | absfs |
| Implementation | Complete filesystem implementations | memfs, osfs, boltfs |
| Adapter | Bridge to external systems | httpfs, s3fs, sftpfs |
| Wrapper | Modify behavior of underlying fs | compressfs, encryptfs, cachefs |
| Compositor | Combine multiple filesystems | unionfs, switchfs |
| Consumer | Use filesystems for specific purposes | ioutil, fstools |

Use `Suite` for Implementation and Adapter types. Use `WrapperSuite` for Wrapper types.
