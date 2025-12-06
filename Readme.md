# fstesting

A test suite for validating `absfs.FileSystem` implementations.

## Usage

```go
import (
    "testing"
    "github.com/absfs/fstesting"
)

func TestMyFS(t *testing.T) {
    fs := NewMyFileSystem()

    suite := &fstesting.Suite{
        FS:       fs,
        Features: fstesting.DefaultFeatures(),
    }

    suite.Run(t)
}
```

## Features

The `Features` struct controls which tests run based on filesystem capabilities:

```go
type Features struct {
    Symlinks      bool // Supports symbolic links
    HardLinks     bool // Supports hard links
    Permissions   bool // Supports chmod/file permissions
    Timestamps    bool // Supports chtimes/modification times
    CaseSensitive bool // Case-sensitive filenames
    AtomicRename  bool // Atomic rename operations
    SparseFiles   bool // Supports sparse files
    LargeFiles    bool // Supports files > 4GB
}
```

Use `DefaultFeatures()` for full OS-like filesystem support, or customize for limited implementations.

## Test Categories

- **FileOperations**: Create, Read, OpenFile, Truncate, Remove, Rename, Stat
- **DirectoryOperations**: Mkdir, MkdirAll, RemoveAll, ReadDir
- **PathHandling**: DotPaths, TrailingSlash, SpecialCharacters, Unicode
- **ErrorSemantics**: NotExist, Exist, IsDir, NotDir
- **Symlinks**: CreateAndRead, Lstat (when supported)
- **Permissions**: Chmod (when supported)
- **Timestamps**: Chtimes (when supported)

## Quick Sanity Check

For fast validation during development:

```go
suite.QuickCheck(t)
```

## Wrapper Testing

For filesystem wrappers (compression, encryption, etc.):

```go
wrapperSuite := &fstesting.WrapperSuite{
    Factory: func(base absfs.FileSystem) (absfs.FileSystem, error) {
        return myWrapper.New(base)
    },
    BaseFS:         baseFS,
    TransformsData: true,  // if wrapper modifies content
}
wrapperSuite.Run(t)
```

## Fuzz Testing

Fuzz test helpers for thorough validation:

```go
func FuzzMyFS(f *testing.F) {
    fs := NewMyFileSystem()
    testDir := fs.TempDir()
    fstesting.FuzzReadWrite(f, fs, testDir)
}
```

Available fuzz tests:
- `FuzzCreate` - File creation with arbitrary paths
- `FuzzReadWrite` - Data integrity through write/read cycles
- `FuzzRename` - Rename operations with arbitrary paths
- `FuzzMkdir` - Directory creation with arbitrary paths
- `FuzzPathTraversal` - Path traversal safety testing
- `FuzzOpenFlags` - OpenFile with various flag combinations

## License

MIT License. See [LICENSE](LICENSE).
