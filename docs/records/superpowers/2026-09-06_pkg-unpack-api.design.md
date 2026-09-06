# Public library API for archive extraction

## Motivation

`unpack` is currently a CLI-only tool: all archive-handling logic lives in `internal/unpack`,
whose exported surface is deliberately zero outside this module (Go's `internal` mechanism
enforces that). Other Go projects that want the same safe-extraction behavior (path-traversal
rejection, symlink-escape checks, smart default destinations, selector-based partial extraction)
currently cannot depend on this code at all.

This document specifies moving the reusable archive logic into a public package,
`pkg/unpack`, that any external Go module can import, while keeping the CLI as a thin consumer
of that package.

## Scope

In scope: relocating and exporting the archive scan/plan/extract/select/download logic; designing
a high-level `Extract` entry point and a set of lower-level primitives; merging the remaining
CLI-only glue into `main.go`; migrating tests accordingly.

Out of scope: any behavior change to extraction semantics, safety checks, or CLI flags. This is a
structural/API refactor of already-working logic, not a feature change.

## Directory layout

- `pkg/unpack/` (new, `package unpack`) receives `archive.go`, `path.go`, `zip.go`, `tar.go`,
  `select.go`, `download.go`, `names.go`, and their `_test.go` files from `internal/unpack/`.
  Identifiers needed by the public API are exported (capitalized); internal helpers stay
  unexported.
- `internal/unpack/` is deleted entirely once its CLI-only contents move to `main.go`.
- `main.go` (root, `package main`) absorbs the former `run.go` (flag parsing, usage text,
  `validateOptionSyntax`, CLI-level orchestration and printing) and `version.go` (`vcsInfo`). It
  keeps an unexported, testable `run(args []string, stdout, stderr io.Writer, workingDir string)
  int` function; `main()` itself only calls `os.Exit(run(os.Args[1:], os.Stdout, os.Stderr,
  workingDir))`. `run_test.go` and `version_test.go` move to the root as `package main` tests.

Rationale: `download.go` and the archive-processing files contain no CLI-specific behavior (no
flag parsing, no printing to stdout/stderr) and are the parts an external consumer actually wants.
Only argument parsing, usage text, and terminal output are CLI-specific and stay out of the
library.

## Public API (`pkg/unpack`)

### Types

Named to avoid stuttering against the package name (`unpack.Entry`, not
`unpack.ArchiveEntry`):

- `type Format uint8` with constants `FormatZIP`, `FormatTAR`, `FormatTarGzip`.
- `type EntryKind uint8` with constants `EntryFile`, `EntryDirectory`, `EntrySymlink`.
- `type Entry struct { Name string; Kind EntryKind; Mode fs.FileMode; LinkTarget string;
  SourceIndex int }` (was `archiveEntry`).
- `type Plan struct { Entry Entry; ArchiveName string; RelativeName string; Destination string;
  TopLevelDir string }` (was `plannedEntry`).

### High-level entry point

```go
type Options struct {
    Directory  string   // extraction base directory; "" keeps the existing smart-default rule
    WorkingDir string   // base for resolving a relative Directory and for the smart default;
                         // "" resolves against the process's current directory
    Chinese    bool     // decode legacy Chinese filenames as GBK
    Overwrite  bool     // overwrite existing files and directories
    Selectors  []string // restrict extraction to matching entries; empty extracts everything
}

type Result struct {
    Directory string   // resolved absolute extraction directory
    Skipped   []string // relative paths skipped because they already existed
}

func Extract(archivePath string, opts Options) (Result, error)
```

`Extract` operates on local file paths only. It runs detect → plan (including the
`Directory`/`WorkingDir` relative-path resolution that today happens in `Run` before calling
`processArchive`) → scan → filter (only when `Options.Selectors` is non-empty) → write, matching
today's `processArchive` behavior, but returns results instead of printing them. It does not
perform URL downloads; a caller that wants to extract from a URL composes `IsRemoteURL` and
`Download` itself, exactly as the CLI does.

`Result.Skipped` is populated with whatever entries were already skipped even when `Extract`
returns a non-nil error from the write phase (matching today's behavior, where `processArchive`
prints already-collected `Skipping existing file:` lines before reporting a later write failure).
Callers that only check `err != nil` and discard `Result` lose nothing they rely on today; callers
that want the partial list on failure can still read `Result.Skipped` alongside a non-nil error.

### Low-level primitives

For callers that want to assemble their own pipeline (for example: scan without writing, or a
custom filter step):

- `func DetectFormat(path string) (Format, error)`
- `func Scan(path string, format Format, chinese bool) ([]Entry, error)`
- `func PlanEntries(entries []Entry, archivePath, directory, workingDir string, format Format)
  ([]Plan, string, error)`
- `func FilterPlans(plans []Plan, selectors []string) ([]Plan, error)`
- `func WritePlans(archivePath string, format Format, plans []Plan, overwrite bool) ([]string,
  error)` (was `extractArchive`; renamed so it isn't confused with the high-level `Extract` — it
  only writes an already-computed `Plan` slice to disk)
- `func IsRemoteURL(candidate string) bool`
- `func Download(rawURL string) (localPath string, cleanup func(), err error)`

Everything else that exists today purely to support these primitives (`normalizeArchivePath`,
`resolveExistingPath`, `requireWithin`, `globToRegexp`, `decodeArchiveName`, `directoryFinalizer`,
`writeNewFile`, `writeSymlink`, ...) stays unexported. These are implementation details of a
primitive, not steps a caller assembles independently.

### Errors

Three sentinel errors are introduced for the recoverable cases a caller plausibly wants to branch
on; every other failure (I/O errors, malformed ZIP/TAR containers, etc.) stays an opaque wrapped
error, since there is no useful branch a caller could take on those besides reporting them:

- `var ErrUnsupportedFormat = errors.New("unsupported archive format")` — returned (wrapped) by
  `DetectFormat` when the filename extension isn't `.zip`, `.tar`, `.tar.gz`, or `.tgz`.
- `var ErrNoMatch = errors.New("selector matched no entries")` — returned (wrapped, with the
  offending selector in the message) by `FilterPlans` when a selector matches nothing.
- `var ErrUnsafeEntry = errors.New("unsafe archive entry")` — returned (wrapped) by every
  "reject before writing" branch: `Scan` for entry kinds ZIP/TAR can't safely represent (hard
  links, devices, FIFOs, encrypted ZIP entries, directory-named symlinks) and undecodable/invalid
  (non-GBK) legacy Chinese filenames; `PlanEntries` for absolute paths, path traversal, and
  symlink-escape targets. Purely internal consistency checks (e.g. a hand-built `Plan` pointing at
  an out-of-range `SourceIndex`, only reachable by a caller misusing the low-level primitives
  directly) stay opaque — they signal a programming error, not an untrusted archive. This lets a
  caller detect "this archive was rejected as unsafe" with a single `errors.Is` check instead of
  parsing error text, regardless of which phase caught the problem.

All three remain wrapped with `fmt.Errorf("...: %w", ...)` at each call site so the specific
message (which selector, which path) is preserved; only the sentinel identity is new.

## `main.go` composition

```
run(args, stdout, stderr, workingDir) int
  → parse flags into archivePath, unpack.Options, selectors
  → if unpack.IsRemoteURL(archivePath): print "Downloading: ...", call unpack.Download, defer cleanup
  → result, err := unpack.Extract(archivePath, opts)
  → print "Extracting: ...", "Output directory: <result.Directory>", one line per result.Skipped
  → on error, print "Error: %v" and return the matching exit code
```

Exit codes and printed message formats are unchanged from today's CLI output.

## Testing

- The bulk of today's `internal/unpack/run_test.go` cases exercise safety and extraction behavior
  (path-traversal rejection, symlink-escape validation, GBK decoding, selector matching, overwrite
  behavior, smart-default directory rules) through `Run(...)`. These move to `pkg/unpack` and are
  rewritten against `Extract(...)` directly.
- `archive_test.go`, `zip_test.go`, `tar_test.go`, `path_test.go`, `select_test.go`,
  `download_test.go`, `names_test.go`, and `test_helpers_test.go` move to `pkg/unpack` unchanged
  in structure, with references to renamed identifiers updated.
- The `main.go`-side `run_test.go` keeps only CLI-layer cases: `--help`/usage text, `--version`
  output, `validateOptionSyntax` edge cases (`--`, undefined single-dash long options), and the
  URL-download-to-CLI-print wiring. It does not re-verify `Extract`'s internal safety logic.

## Non-goals / explicitly deferred

- No change to any CLI flag, printed message, or extraction/safety semantics.
- No semantic versioning or API-stability policy for `pkg/unpack` is established by this change;
  that is a separate decision if/when this becomes a published, externally-consumed module.
- No `go.dev`-style package documentation pass beyond GoDoc comments on newly exported symbols.
