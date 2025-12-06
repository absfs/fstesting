# fstesting

Comprehensive test suite for [absfs](https://github.com/absfs/absfs) filesystem implementations.

```go
func TestMyFS(t *testing.T) {
    suite := &fstesting.Suite{
        FS:       myFS,
        Features: fstesting.DefaultFeatures(),
    }
    suite.Run(t)
}
```

That's it. One struct, one method call, and you get 30+ tests covering file operations, directories, path handling, error semantics, symlinks, permissions, and timestamps.

## Features

- **Baseline Tests** - Standard tests that any filesystem should pass
- **Feature Flags** - Skip tests for unsupported capabilities (symlinks, permissions, etc.)
- **Fuzz Testing** - Property-based tests for edge cases and security
- **Wrapper Testing** - Specialized suite for filesystem wrappers (compression, encryption)
- **Quick Check** - Fast sanity test for development iterations

## Install

```bash
go get github.com/absfs/fstesting
```

## Documentation

- [Guide](GUIDE.md) - Detailed usage, configuration, and examples
- [GoDoc](https://pkg.go.dev/github.com/absfs/fstesting) - API reference

## License

MIT
