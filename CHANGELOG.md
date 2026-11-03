# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- `Skip` type and `Suite.Skips` field: skip individual suite subtests by name
  or by path below the suite root, each with a reason. Skipped subtests call
  `t.Skip` with a message naming the subtest, the matching pattern, and the
  reason, so they show up as `SKIP` in `go test -v`.
- Skip patterns are relative to the `*testing.T` passed to `Run` or
  `RunWithSkips`, so they stay the same when the suite runs inside a subtest.
- Skip patterns that match no subtest are reported with `t.Logf`, naming the
  test they are relative to and suggesting a fix when a pattern repeats that
  test's name. Path patterns below a skipped group, and duplicates, count as
  matched. Empty or malformed patterns fail the test before any subtest runs.
- `CHANGELOG.md`.

### Fixed

- `Suite.RunWithSkips` ignored its arguments and ran every subtest. It now
  skips exactly the subtests its patterns select (see `Skip` for the rules),
  in addition to any in `Suite.Skips`, which it leaves unmodified. Consumers
  no longer need a `TestMain`
  that sets `-test.skip`, or a shim, to skip one subtest.
- `Symlinks/Lstat` panicked with a nil-pointer dereference when `Stat` on the
  link failed, for example over a route that rewrites symlink targets. It now
  fails with the `Stat` error.
- `Suite` no longer panics on other operations that fail where it previously
  ignored the error (`Create`, `Open`, `Stat`, `Lstat`, `Symlink` during
  setup). Each reports the failing operation, path, and error instead.
- `Suite` and `WrapperSuite` now check `Write`, `Close`, and read errors when
  writing and reading back test files, instead of comparing whatever was
  read.

## [1.0.0]

- Update to absfs v1.0.0 dependencies.

[Unreleased]: https://github.com/absfs/fstesting/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/absfs/fstesting/releases/tag/v1.0.0
