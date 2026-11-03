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
- Skip patterns that match no subtest are reported with `t.Logf`; empty or
  malformed patterns fail the test before any subtest runs.
- `CHANGELOG.md`.

### Fixed

- `Suite.RunWithSkips` ignored its arguments and ran every subtest. It now
  skips exactly the subtests its patterns select (see `Skip` for the rules),
  in addition to any in `Suite.Skips`. Consumers no longer need a `TestMain`
  that sets `-test.skip`, or a shim, to skip one subtest.
- `Symlinks/Lstat` panicked with a nil-pointer dereference when `Stat` on the
  link failed, for example over a route that rewrites symlink targets. It now
  fails with the `Stat` error.
- The suites no longer panic on other operations that fail where they
  previously ignored the error (`Create`, `Open`, `Stat`, `Lstat`, `Symlink`
  during setup). Each reports the failing operation, path, and error instead.
  Setup writes now also check `Write` and `Close` errors.

## [1.0.0]

- Update to absfs v1.0.0 dependencies.

[Unreleased]: https://github.com/absfs/fstesting/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/absfs/fstesting/releases/tag/v1.0.0
