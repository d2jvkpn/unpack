# Repository Guidelines

## Project Structure & Module Organization

This repository contains a small Go command-line application for safely extracting ZIP, TAR,
TAR.GZ, and TGZ archives, built on top of `pkg/unpack`, a public library package any Go module can
import. At the repository root (`package main`), `main.go` resolves the working directory and
calls `run`; `cli.go` holds flag parsing, usage text, and CLI-level orchestration; `version.go`
reports build/VCS metadata for `--version`. Within `pkg/unpack`, `extract.go` exposes the
high-level `Extract` entry point; `archive.go` handles format detection and dispatch; `zip.go` and
`tar.go` implement per-format scanning and writing; `path.go` validates destinations and builds
extraction plans; `select.go` matches file selectors; `download.go` fetches archives from
http(s) URLs; `names.go` decodes legacy Chinese filenames; and `errors.go` defines the sentinel
errors (`ErrUnsupportedFormat`, `ErrNoMatch`, `ErrUnsafeEntry`) library callers can match with
`errors.Is`. Tests are colocated with their implementation as `*_test.go`. `unzip_cn.py` and
`test_unzip_cn.py` provide the legacy Python implementation and its tests. Design records are
stored under `docs/records/superpowers/`.

## Build, Test, and Development Commands

- `go build -o unpack .` builds the local CLI binary.
- `go install .` installs the command into the configured Go binary directory.
- `go test ./...` runs the complete Go test suite.
- `go test -run TestRun ./...` runs a focused group of tests while iterating.
- `python3 -m unittest test_unzip_cn.py` tests the Python compatibility utility.
- `gofmt -w .` formats Go source and tests before review.

Go 1.27 or newer is required. Run the built binary with commands such as
`./unpack --directory restored archive.tar.gz`.

## Coding Style & Naming Conventions

Follow standard Go formatting and idioms: tabs as produced by `gofmt`, short package-local names,
and PascalCase only for exported identifiers. Keep format-specific logic in its corresponding file
and preserve the scan-before-write safety model. Name tests descriptively with the
`Test<Behavior>` pattern, for example `TestPlanEntriesRejectsTraversal`.

## Testing Guidelines

Use Go's `testing` package and temporary directories or synthetic archives to keep tests isolated.
Add regression tests for parsing, filename decoding, destination planning, archive validation, and
extraction failures. Safety-sensitive changes must verify that invalid archives are rejected before
payload files are written and that existing files are only overwritten when `--overwrite` is
passed.

## Commit & Pull Request Guidelines

Recent history uses Conventional Commit-style subjects such as
`feat: add safe configurable zip extraction`. Use an imperative, concise subject with an
appropriate prefix (`feat:`, `fix:`, `test:`, or `docs:`). Pull requests should explain
user-visible behavior, call out security or compatibility implications, link related issues, and
list verification commands. Update `README.md` when CLI flags, supported formats, output rules, or
requirements change.

@./.agents/me/README.md
