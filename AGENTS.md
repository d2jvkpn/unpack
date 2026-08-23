# Repository Guidelines

@./.agents/me/Docs.md

## Project Structure & Module Organization

This repository contains a small Go command-line application for safely extracting ZIP, TAR,
TAR.GZ, and TGZ archives. Command orchestration and argument parsing live in `main.go`; archive
format detection is in `archive.go`; ZIP and TAR handling are separated into `zip.go` and `tar.go`;
path validation and extraction planning live in `path.go`; and filename decoding helpers are in
`names.go`. Tests are colocated with their implementation as `*_test.go`. `unzip_cn.py` and
`test_unzip_cn.py` provide the legacy Python implementation and its tests. Design records are
stored under `docs/records/superpowers/`.

## Build, Test, and Development Commands

- `go build -o unpack .` builds the local CLI binary.
- `go install .` installs the command into the configured Go binary directory.
- `go test ./...` runs the complete Go test suite.
- `go test -run TestRun ./...` runs a focused group of tests while iterating.
- `python3 -m unittest test_unzip_cn.py` tests the Python compatibility utility.
- `gofmt -w *.go` formats Go source and tests before review.

Go 1.27 or newer is required. Run the built binary with commands such as
`./unpack --output-dir restored archive.tar.gz`.

## Coding Style & Naming Conventions

Follow standard Go formatting and idioms: tabs as produced by `gofmt`, short package-local names,
and PascalCase only for exported identifiers. Keep format-specific logic in its corresponding file
and preserve the scan-before-write safety model. Name tests descriptively with the
`Test<Behavior>` pattern, for example `TestPlanEntriesRejectsTraversal`.

## Testing Guidelines

Use Go's `testing` package and temporary directories or synthetic archives to keep tests isolated.
Add regression tests for parsing, filename decoding, destination planning, archive validation, and
extraction failures. Safety-sensitive changes must verify that invalid archives are rejected before
payload files are written and that existing files are never overwritten.

## Commit & Pull Request Guidelines

Recent history uses Conventional Commit-style subjects such as
`feat: add safe configurable zip extraction`. Use an imperative, concise subject with an
appropriate prefix (`feat:`, `fix:`, `test:`, or `docs:`). Pull requests should explain
user-visible behavior, call out security or compatibility implications, link related issues, and
list verification commands. Update `README.md` when CLI flags, supported formats, output rules, or
requirements change.

@.agents/me/Docs.md
