# Public library API for archive extraction — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the reusable archive scan/plan/extract/select/download logic out of
`internal/unpack` into a new, externally importable `pkg/unpack` package with a small high-level
`Extract` entry point plus low-level primitives, and reduce the CLI to a thin consumer of that
package living directly in `package main`.

**Architecture:** `pkg/unpack` becomes the single source of truth for archive handling, exporting
`Format`, `EntryKind`, `Entry`, `Plan`, three sentinel errors, a high-level `Extract(archivePath,
Options) (Result, error)`, and low-level primitives (`DetectFormat`, `Scan`, `PlanEntries`,
`FilterPlans`, `WritePlans`, `IsRemoteURL`, `Download`). `internal/unpack` is deleted; its
CLI-only glue (flag parsing, usage text, printing, `--version` support) moves into `main.go` +
new `cli.go`/`version.go` files at the module root, all `package main`.

**Tech Stack:** Go 1.27, standard library only for the moved code (`archive/zip`, `archive/tar`,
`net/http`, ...), `golang.org/x/text` for GBK decoding (already a dependency).

**Spec:** `docs/records/superpowers/2026-09-06_pkg-unpack-api.design.md`

## Global Constraints

- No behavior change to CLI flags, printed messages, exit codes, or extraction/safety semantics —
  this is a structural/API refactor of already-working logic.
- Go 1.27; `gofmt -w .` and `go vet ./...` must stay clean; run `go test ./...` after every task.
- Follow `.agents/me/my-go-dev.md`: `var`-block declarations (steps below already respect this
  since they only touch signatures/wrapping, not control flow), GoDoc comment on every newly
  exported symbol, 120-char line limit, `New*` constructor naming, `fmt.Errorf("verb noun: %w",
  err)` phrasing.
- Because every file being moved in Task 1 refers to the same package-local types
  (`Entry`/`Plan`/`Format`/...), `pkg/unpack` only compiles once *all* of Task 1's files are in
  place — the task is intentionally one atomic unit ending in a single build+test+commit, not one
  gate per file. Task 2 (the CLI cutover) is similarly atomic: `main.go` can't switch imports
  until `internal/unpack` is gone and `pkg/unpack` exists, and vice versa.
- This plan uses exact `git mv` / `sed` commands for the mechanical parts (moving files, renaming
  identifiers everywhere they're used) rather than reproducing every unchanged function body —
  the commands themselves are the concrete, copy-pasteable content for those steps. Every
  *semantic* change (new sentinel errors, new files, doc comments, error-wrapping edits, test
  assertions) is given as an exact diff.

---

## Task 1: Create `pkg/unpack`

**Files:**
- Create: `pkg/unpack/archive.go`, `pkg/unpack/path.go`, `pkg/unpack/zip.go`, `pkg/unpack/tar.go`,
  `pkg/unpack/select.go`, `pkg/unpack/download.go`, `pkg/unpack/names.go`, `pkg/unpack/errors.go`,
  `pkg/unpack/extract.go` (all new via `git mv` + edits, except `errors.go`/`extract.go` which are
  wholly new)
- Create (tests): `pkg/unpack/archive_test.go`, `pkg/unpack/path_test.go`, `pkg/unpack/zip_test.go`,
  `pkg/unpack/tar_test.go`, `pkg/unpack/select_test.go`, `pkg/unpack/download_test.go`,
  `pkg/unpack/names_test.go`, `pkg/unpack/test_helpers_test.go`, `pkg/unpack/extract_test.go` (new)
- Leave untouched for now: `internal/unpack/*` (deleted in Task 2), `main.go`

**Interfaces:**
- Produces (for Task 2 to consume):
  - `type unpack.Options struct { Directory, WorkingDir string; Chinese, Overwrite bool;
    Selectors []string }`
  - `type unpack.Result struct { Directory string; Skipped []string }`
  - `func unpack.Extract(archivePath string, opts unpack.Options) (unpack.Result, error)`
  - `func unpack.IsRemoteURL(candidate string) bool`
  - `func unpack.Download(rawURL string) (localPath string, cleanup func(), err error)`
  - `var unpack.ErrUnsupportedFormat, unpack.ErrNoMatch, unpack.ErrUnsafeEntry error`

- [ ] **Step 1: Move the implementation and test files**

```bash
mkdir -p pkg/unpack
git mv internal/unpack/archive.go pkg/unpack/archive.go
git mv internal/unpack/archive_test.go pkg/unpack/archive_test.go
git mv internal/unpack/path.go pkg/unpack/path.go
git mv internal/unpack/path_test.go pkg/unpack/path_test.go
git mv internal/unpack/zip.go pkg/unpack/zip.go
git mv internal/unpack/zip_test.go pkg/unpack/zip_test.go
git mv internal/unpack/tar.go pkg/unpack/tar.go
git mv internal/unpack/tar_test.go pkg/unpack/tar_test.go
git mv internal/unpack/select.go pkg/unpack/select.go
git mv internal/unpack/select_test.go pkg/unpack/select_test.go
git mv internal/unpack/download.go pkg/unpack/download.go
git mv internal/unpack/download_test.go pkg/unpack/download_test.go
git mv internal/unpack/names.go pkg/unpack/names.go
git mv internal/unpack/names_test.go pkg/unpack/names_test.go
git mv internal/unpack/test_helpers_test.go pkg/unpack/test_helpers_test.go
```

- [ ] **Step 2: Rename identifiers to their exported names, everywhere in `pkg/unpack`**

Run this from the repo root. Each substitution uses word boundaries so it only touches the exact
identifier (e.g. it will not touch `formatTarGzip` while renaming `formatTAR`):

```bash
cd pkg/unpack
sed -i \
  -e 's/\barchiveFormat\b/Format/g' \
  -e 's/\bformatZIP\b/FormatZIP/g' \
  -e 's/\bformatTAR\b/FormatTAR/g' \
  -e 's/\bformatTarGzip\b/FormatTarGzip/g' \
  -e 's/\bentryKind\b/EntryKind/g' \
  -e 's/\bentryFile\b/EntryFile/g' \
  -e 's/\bentryDirectory\b/EntryDirectory/g' \
  -e 's/\bentrySymlink\b/EntrySymlink/g' \
  -e 's/\barchiveEntry\b/Entry/g' \
  -e 's/\bplannedEntry\b/Plan/g' \
  -e 's/\bdetectFormat\b/DetectFormat/g' \
  -e 's/\bscanArchive\b/Scan/g' \
  -e 's/\bextractArchive\b/WritePlans/g' \
  -e 's/\bplanEntries\b/PlanEntries/g' \
  -e 's/\bfilterPlans\b/FilterPlans/g' \
  -e 's/\bdownloadArchive\b/Download/g' \
  -e 's/\bisRemoteURL\b/IsRemoteURL/g' \
  *.go
cd ../..
```

- [ ] **Step 3: Update the package doc comment and add GoDoc comments for newly exported symbols**

In `pkg/unpack/archive.go`, add a package doc comment and comments on the exported declarations:

```go
// Package unpack safely extracts ZIP, TAR, TAR.GZ, and TGZ archives: it validates every entry's
// destination before writing anything to disk, rejecting path traversal, symlink escapes, and
// unsupported entry types. Extract is the high-level entry point; DetectFormat, Scan,
// PlanEntries, FilterPlans, and WritePlans are lower-level primitives for callers that want to
// assemble their own pipeline.
package unpack
```

Immediately above `type Format uint8`, add:

```go
// Format identifies which archive container a path was detected as.
```

Immediately above the `const ( FormatZIP Format = iota + 1 ...` block, add:

```go
// Supported archive formats.
```

Immediately above `type EntryKind uint8`, add:

```go
// EntryKind identifies what an archive entry represents on disk.
```

Immediately above the `const ( EntryFile EntryKind = iota + 1 ...` block, add:

```go
// Supported entry kinds.
```

Immediately above `type Entry struct`, add:

```go
// Entry is a single archive member discovered by Scan.
```

Immediately above `func DetectFormat(path string) (Format, error)`, add:

```go
// DetectFormat identifies an archive's format from its filename suffix. It returns
// ErrUnsupportedFormat, wrapped with the rejected filename, when the suffix isn't recognized.
```

Immediately above `func Scan(path string, format Format, chinese bool) ([]Entry, error)`, add:

```go
// Scan reads every entry in the archive at path without writing anything to disk. It returns
// ErrUnsafeEntry, wrapped with details, for entry kinds this package can't safely represent
// (hard links, devices, FIFOs, encrypted ZIP entries, directory-named symlinks) and for
// undecodable legacy Chinese filenames.
```

Immediately above `func WritePlans(path string, format Format, plans []Plan, overwrite bool)
([]string, error)`, add:

```go
// WritePlans writes the given plans, as produced by PlanEntries, to disk. It returns the
// archive-relative names of entries that already existed and were preserved (overwrite is
// false), and does so even when it also returns a non-nil error: entries written or skipped
// before a later failure are still reported.
```

In `pkg/unpack/path.go`, immediately above `type Plan struct`, add:

```go
// Plan is a single archive entry paired with its validated destination path.
```

Immediately above `func PlanEntries(entries []Entry, archivePath, outputDir, workingDir string,
format Format) ([]Plan, string, error)` (the parameter is still named `outputDir`, unchanged by
the Step 2 rename — only the package-level identifiers `planEntries`/`archiveEntry`/`plannedEntry`
/`archiveFormat` were renamed, not this parameter), add:

```go
// PlanEntries validates every entry and computes its destination path, resolving the smart
// default destination when outputDir is empty. It returns ErrUnsafeEntry, wrapped with details,
// before anything is written to disk: absolute paths, path traversal, and symlink-escape targets
// are all rejected here.
```

In `pkg/unpack/select.go`, immediately above `func FilterPlans(plans []Plan, selectors []string)
([]Plan, error)`, add:

```go
// FilterPlans restricts plans to those matching at least one selector (exact path, directory
// prefix, or glob — see selectorMatches). It returns ErrNoMatch, wrapped with the offending
// selector, if any selector matches nothing.
```

In `pkg/unpack/download.go`, immediately above `func IsRemoteURL(candidate string) bool`, add:

```go
// IsRemoteURL reports whether candidate looks like an http(s) URL rather than a local file path.
```

Immediately above `func Download(rawURL string) (string, func(), error)`, add:

```go
// Download fetches rawURL into a temporary file and returns its local path and a cleanup
// function that removes the temporary file and its containing directory. The caller is
// responsible for calling cleanup once done with the file.
```

- [ ] **Step 4: Add the sentinel errors**

Create `pkg/unpack/errors.go`:

```go
package unpack

import "errors"

// ErrUnsupportedFormat is returned, wrapped, by DetectFormat when a filename's extension isn't
// one of the supported archive formats.
var ErrUnsupportedFormat = errors.New("unsupported archive format")

// ErrNoMatch is returned, wrapped, by FilterPlans when a selector matches no entries.
var ErrNoMatch = errors.New("selector matched no entries")

// ErrUnsafeEntry is returned, wrapped, by Scan and PlanEntries when an archive entry is rejected
// before anything is written to disk: an entry kind this package can't safely represent, an
// undecodable filename, an absolute or traversing path, or a symlink-escape target.
var ErrUnsafeEntry = errors.New("unsafe archive entry")
```

- [ ] **Step 5: Wrap `ErrUnsupportedFormat` at its call site**

In `pkg/unpack/archive.go`, inside `DetectFormat`:

```go
	default:
		return 0, fmt.Errorf("unsupported archive format: %s", filepath.Base(path))
	}
```

becomes:

```go
	default:
		return 0, fmt.Errorf("%w: %s", ErrUnsupportedFormat, filepath.Base(path))
	}
```

- [ ] **Step 6: Wrap `ErrNoMatch` at its call site**

In `pkg/unpack/select.go`, inside `FilterPlans`:

```go
	for i, selector := range selectors {
		if !matched[i] {
			return nil, fmt.Errorf("selector %q matched no entries", selector)
		}
	}
```

becomes:

```go
	for i, selector := range selectors {
		if !matched[i] {
			return nil, fmt.Errorf("%w: %q", ErrNoMatch, selector)
		}
	}
```

- [ ] **Step 7: Wrap `ErrUnsafeEntry` at its call sites in `zip.go`**

In `pkg/unpack/zip.go`, inside `scanZIP` (formerly the function under `func scanZIP`, unrenamed
since it stays package-private):

```go
		if file.Flags&0x0001 != 0 {
			return nil, fmt.Errorf("unsafe ZIP entry %q: encrypted entries are unsupported", file.Name)
		}
		name, err := decodeArchiveName(file.Name, chinese, file.Flags&0x800 == 0)
		if err != nil {
			return nil, fmt.Errorf("decode ZIP entry %q: %w", file.Name, err)
		}

		kind, err := zipEntryKind(file)
		if err != nil {
			return nil, fmt.Errorf("unsafe ZIP entry %q: %w", name, err)
		}
		var linkTarget string
		if kind == EntrySymlink {
			linkTarget, err = readZIPSymlinkTarget(file)
			if err != nil {
				return nil, fmt.Errorf("unsafe ZIP entry %q: %w", name, err)
			}
		}
```

becomes:

```go
		if file.Flags&0x0001 != 0 {
			return nil, fmt.Errorf("%w: %q: encrypted entries are unsupported", ErrUnsafeEntry, file.Name)
		}
		name, err := decodeArchiveName(file.Name, chinese, file.Flags&0x800 == 0)
		if err != nil {
			return nil, fmt.Errorf("%w: decode ZIP entry %q: %w", ErrUnsafeEntry, file.Name, err)
		}

		kind, err := zipEntryKind(file)
		if err != nil {
			return nil, fmt.Errorf("%w: %q: %w", ErrUnsafeEntry, name, err)
		}
		var linkTarget string
		if kind == EntrySymlink {
			linkTarget, err = readZIPSymlinkTarget(file)
			if err != nil {
				return nil, fmt.Errorf("%w: %q: %w", ErrUnsafeEntry, name, err)
			}
		}
```

- [ ] **Step 8: Wrap `ErrUnsafeEntry` at its call site in `tar.go`**

In `pkg/unpack/tar.go`, inside `scanTAR`:

```go
		name, decodeErr := decodeArchiveName(header.Name, chinese, !utf8.ValidString(header.Name))
		if decodeErr != nil {
			return nil, fmt.Errorf("decode TAR entry %q: %w", header.Name, decodeErr)
		}

		kind := EntryKind(0)
		var linkTarget string
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			kind = EntryFile
		case tar.TypeDir:
			kind = EntryDirectory
		case tar.TypeSymlink:
			kind = EntrySymlink
			linkTarget = header.Linkname
		default:
			return nil, fmt.Errorf("unsafe TAR entry %q: unsupported type %q", name, header.Typeflag)
		}
```

becomes:

```go
		name, decodeErr := decodeArchiveName(header.Name, chinese, !utf8.ValidString(header.Name))
		if decodeErr != nil {
			return nil, fmt.Errorf("%w: decode TAR entry %q: %w", ErrUnsafeEntry, header.Name, decodeErr)
		}

		kind := EntryKind(0)
		var linkTarget string
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			kind = EntryFile
		case tar.TypeDir:
			kind = EntryDirectory
		case tar.TypeSymlink:
			kind = EntrySymlink
			linkTarget = header.Linkname
		default:
			return nil, fmt.Errorf("%w: %q: unsupported type %q", ErrUnsafeEntry, name, header.Typeflag)
		}
```

- [ ] **Step 9: Wrap `ErrUnsafeEntry` at its call sites in `path.go`**

In `pkg/unpack/path.go`, inside `PlanEntries`, apply these five edits:

Edit 9a — unsupported entry kind and normalize-path failure:

```go
		switch entry.Kind {
		case EntryFile, EntryDirectory, EntrySymlink:
		default:
			return nil, "", fmt.Errorf("unsafe archive entry %q: unsupported entry kind %d", entry.Name, entry.Kind)
		}
		name, err := normalizeArchivePath(entry.Name)
		if err != nil {
			return nil, "", fmt.Errorf("unsafe archive entry %q: %w", entry.Name, err)
		}
```

becomes:

```go
		switch entry.Kind {
		case EntryFile, EntryDirectory, EntrySymlink:
		default:
			return nil, "", fmt.Errorf("%w: %q: unsupported entry kind %d", ErrUnsafeEntry, entry.Name, entry.Kind)
		}
		name, err := normalizeArchivePath(entry.Name)
		if err != nil {
			return nil, "", fmt.Errorf("%w: %q: %w", ErrUnsafeEntry, entry.Name, err)
		}
```

Edit 9b — root marker validation:

```go
		if name == "." {
			if entry.Kind != EntryDirectory {
				return nil, "", fmt.Errorf("unsafe archive entry %q: root marker is not a directory", entry.Name)
			}
```

becomes:

```go
		if name == "." {
			if entry.Kind != EntryDirectory {
				return nil, "", fmt.Errorf("%w: %q: root marker is not a directory", ErrUnsafeEntry, entry.Name)
			}
```

Edit 9c — top-level ancestor checks:

```go
	if err := rejectFileTopLevelAncestors(names, entries); err != nil {
		return nil, "", err
	}
	if err := rejectSymlinkAncestors(names, entries); err != nil {
		return nil, "", err
	}
```

becomes:

```go
	if err := rejectFileTopLevelAncestors(names, entries); err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrUnsafeEntry, err)
	}
	if err := rejectSymlinkAncestors(names, entries); err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrUnsafeEntry, err)
	}
```

Edit 9d — symlink target validation and synthetic base name safety:

```go
		if err := validateSymlinkTarget(names[i], entry.LinkTarget); err != nil {
			return nil, "", fmt.Errorf("unsafe archive entry %q: %w", entry.Name, err)
		}
```

becomes:

```go
		if err := validateSymlinkTarget(names[i], entry.LinkTarget); err != nil {
			return nil, "", fmt.Errorf("%w: %q: %w", ErrUnsafeEntry, entry.Name, err)
		}
```

and:

```go
			if !filepath.IsLocal(stem) || filepath.Clean(stem) == "." {
				return nil, "", fmt.Errorf("unsafe archive base name %q", stem)
			}
```

becomes:

```go
			if !filepath.IsLocal(stem) || filepath.Clean(stem) == "." {
				return nil, "", fmt.Errorf("%w: archive base name %q", ErrUnsafeEntry, stem)
			}
```

Edit 9e — per-entry destination containment checks:

```go
		resolvedDestination, err := resolveExistingPath(destination)
		if err != nil {
			return nil, "", fmt.Errorf("unsafe archive entry %q: resolve destination: %w", entry.Name, err)
		}
		if err := requireWithin(resolvedBase, resolvedDestination); err != nil {
			return nil, "", fmt.Errorf("unsafe archive entry %q: %w", entry.Name, err)
		}
```

becomes:

```go
		resolvedDestination, err := resolveExistingPath(destination)
		if err != nil {
			return nil, "", fmt.Errorf("%w: %q: resolve destination: %w", ErrUnsafeEntry, entry.Name, err)
		}
		if err := requireWithin(resolvedBase, resolvedDestination); err != nil {
			return nil, "", fmt.Errorf("%w: %q: %w", ErrUnsafeEntry, entry.Name, err)
		}
```

(The synthetic-default working-directory escape check right above these, and the two
`filepath.Abs`/`resolveExistingPath` calls that resolve `base`/`workingDir` themselves, are left
as opaque errors: they signal a filesystem resolution failure rather than a judgment about the
archive's contents, so `ErrUnsafeEntry` doesn't apply to them.)

- [ ] **Step 10: Add the high-level `Extract` API**

Create `pkg/unpack/extract.go`:

```go
package unpack

import (
	"fmt"
	"path/filepath"
)

// Options configures Extract.
type Options struct {
	// Directory is the extraction base directory. Empty keeps the smart-default rule: a single
	// top-level archive item extracts directly into WorkingDir, multiple top-level items extract
	// into a directory named after the archive.
	Directory string
	// WorkingDir is the base used to resolve a relative Directory and to compute the smart
	// default. Empty resolves against the process's current directory.
	WorkingDir string
	// Chinese decodes legacy (non-UTF-8) filenames as GBK.
	Chinese bool
	// Overwrite replaces existing files and directories at the destination instead of skipping
	// them.
	Overwrite bool
	// Selectors restricts extraction to entries matching at least one selector (see
	// FilterPlans). Empty extracts everything.
	Selectors []string
}

// Result reports what Extract did.
type Result struct {
	// Directory is the resolved, absolute extraction directory.
	Directory string
	// Skipped lists the archive-relative names of entries that already existed and were
	// preserved. It is populated even when Extract also returns a non-nil error from the write
	// phase.
	Skipped []string
}

// Extract detects, plans, and writes every matching entry from the local archive at archivePath
// to disk, applying opts. It does not accept remote URLs; callers that want to extract from a
// URL should compose IsRemoteURL and Download themselves before calling Extract.
func Extract(archivePath string, opts Options) (Result, error) {
	var (
		format  Format
		entries []Entry
		plans   []Plan
		base    string
		skipped []string
		result  Result
		err     error
	)

	format, err = DetectFormat(archivePath)
	if err != nil {
		return result, fmt.Errorf("extract archive %q: %w", archivePath, err)
	}

	directory := opts.Directory
	if directory != "" && !filepath.IsAbs(directory) {
		directory = filepath.Join(opts.WorkingDir, directory)
	}

	entries, err = Scan(archivePath, format, opts.Chinese)
	if err != nil {
		return result, err
	}
	plans, base, err = PlanEntries(entries, archivePath, directory, opts.WorkingDir, format)
	if err != nil {
		return result, fmt.Errorf("plan archive %q: %w", archivePath, err)
	}
	result.Directory = base

	if len(opts.Selectors) > 0 {
		plans, err = FilterPlans(plans, opts.Selectors)
		if err != nil {
			return result, fmt.Errorf("select files in archive %q: %w", archivePath, err)
		}
	}

	skipped, err = WritePlans(archivePath, format, plans, opts.Overwrite)
	result.Skipped = skipped
	if err != nil {
		return result, err
	}
	return result, nil
}
```

- [ ] **Step 11: Migrate the library-level behavior tests out of `run_test.go` into `extract_test.go`**

Create `pkg/unpack/extract_test.go`, rewriting each case listed below against `Extract(...)`
directly instead of `Run(...)`. Copy each test body from
`internal/unpack/run_test.go`(as it stood before this refactor — check it out with
`git show HEAD:internal/unpack/run_test.go` if it's already been deleted by the time you reach
this step) and mechanically replace the `Run([]string{...}, &stdout, &stderr, workingDir)` call
and its status-code assertion with `Extract(archivePath, Options{...})` and an error assertion, per
this example:

```go
package unpack

import (
	"archive/tar"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractUsesSmartDefaultDestinations(t *testing.T) {
	t.Run("one top-level item extracts directly", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "archives", "single.zip")
		if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
			t.Fatal(err)
		}
		writeZIPFixture(t, archivePath, []zipFixture{{Name: "note.txt", Body: "single"}})
		workingDir := filepath.Join(root, "working")
		if err := os.Mkdir(workingDir, 0o755); err != nil {
			t.Fatal(err)
		}

		_, err := Extract(archivePath, Options{WorkingDir: workingDir})

		if err != nil {
			t.Fatalf("Extract() error = %v", err)
		}
		assertFileContents(t, filepath.Join(workingDir, "note.txt"), "single")
		assertPathDoesNotExist(t, filepath.Join(workingDir, "single"))
	})

	// ... "ZIP with multiple top-level items uses archive basename" and
	// "TAR.GZ removes the complete recognized suffix" subtests follow the same
	// transformation: Run([]string{archivePath}, ...) with WorkingDir: workingDir becomes
	// Extract(archivePath, Options{WorkingDir: workingDir}).
}
```

Apply the same `Run(...)` → `Extract(...)` transformation, moving into `extract_test.go`, for
these test functions (drop the `TestRun` prefix, use `TestExtract...` instead; a status code of
`0` becomes `err == nil`, a non-zero status becomes `err != nil`, and `--directory VALUE` in the
args becomes `Directory: VALUE` in `Options`):

- `TestRunUsesSmartDefaultDestinations` → `TestExtractUsesSmartDefaultDestinations` (shown above)
- `TestRunRejectsUnsafeSyntheticDefaultBases` → `TestExtractRejectsUnsafeSyntheticDefaultBases`
- `TestRunRejectsSyntheticDefaultBaseSymlinkEscape` →
  `TestExtractRejectsSyntheticDefaultBaseSymlinkEscape`
- `TestRunAcceptsRootDirectoryMarkers` → `TestExtractAcceptsRootDirectoryMarkers`
- `TestRunExplicitOutputStripsSoleTopLevelDirectory` →
  `TestExtractExplicitOutputStripsSoleTopLevelDirectory`
- `TestRunExtractsBothSupportedFormats` → `TestExtractSupportsBothFormats`
- `TestRunExtractsOnlySelectedFiles` → `TestExtractOnlySelectedFiles` (sets `Selectors:
  []string{"first.txt", "docs"}` in `Options`)
- `TestRunSelectorMatchesRelativeToSoleTopLevelDirectory` →
  `TestExtractSelectorMatchesRelativeToSoleTopLevelDirectory`
- `TestRunFailsWithoutWritingWhenSelectorMatchesNothing` →
  `TestExtractFailsWithoutWritingWhenSelectorMatchesNothing` — additionally assert
  `errors.Is(err, ErrNoMatch)` after the existing failure check
- `TestRunRejectsUnsafeArchivesBeforeWritingPayloads` →
  `TestExtractRejectsUnsafeArchivesBeforeWritingPayloads` — additionally assert
  `errors.Is(err, ErrUnsafeEntry)` in every subtest except `"encrypted ZIP"` and `"TAR hard
  link"`, which already get equivalent coverage from Step 12/13 below; keep all five subtests
  here regardless, since this is the one place that exercises them end-to-end through `Extract`
- `TestRunExtractsSafeSymlinks` → `TestExtractWritesSafeSymlinks`
- `TestRunReportsProgressAndPreservesExistingFiles` →
  `TestExtractReportsSkippedExistingFilesInResult` — replace the `stdout` string assertions with
  `result.Skipped` assertions: `if len(result.Skipped) != 1 || result.Skipped[0] != "note.txt" {
  t.Fatalf("Skipped = %#v", result.Skipped) }`
- `TestRunReportsSkipsBeforeLaterExtractionFailure` →
  `TestExtractReportsSkippedFilesInResultOnLaterFailure` — drop the `triggeringWriter` (it existed
  only to interleave a filesystem-clobbering side effect with `stdout` writes, which no longer
  exist here); instead write `blocked` as a regular file up front, call `Extract`, and assert both
  `err != nil` and `result.Skipped` contains `"first.txt"`

Do **not** move `TestRunShowsHelpAndParsesSupportedFlags`, `TestRunRequiresAtLeastOneArchive`,
`TestRunRejectsShortOutputAlias`, `TestRunRejectsSingleDashLongOptionsWithoutWriting`,
`TestRunAcceptsDashPrefixedOutputDirectoryOperand`, `TestRunStopsOptionParsingAtFirstArchive`,
`TestRunDownloadsArchiveFromURL`, `TestRunReportsErrorForMissingArchive`,
`TestRunTreatsTrailingArchivePathAsSelectorNotSecondArchive` — these test CLI argument parsing and
stay in `internal/unpack/run_test.go` today and move to the root `cli_test.go` in Task 2.

- [ ] **Step 12: Add `ErrUnsupportedFormat`/`ErrNoMatch`/`ErrUnsafeEntry` assertions to the existing unit tests**

In `pkg/unpack/archive_test.go`, in `TestDetectFormatRejectsUnsupportedExtension`:

```go
func TestDetectFormatRejectsUnsupportedExtension(t *testing.T) {
	if _, err := DetectFormat("a.rar"); err == nil {
		t.Fatal("DetectFormat(a.rar) succeeded")
	}
}
```

becomes:

```go
func TestDetectFormatRejectsUnsupportedExtension(t *testing.T) {
	_, err := DetectFormat("a.rar")
	if err == nil {
		t.Fatal("DetectFormat(a.rar) succeeded")
	}
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("DetectFormat(a.rar) error = %v, want ErrUnsupportedFormat", err)
	}
}
```

Add `"errors"` to `pkg/unpack/archive_test.go`'s import block (it currently only imports
`"testing"`).

In `pkg/unpack/select_test.go`, in `TestFilterPlansErrorsOnUnmatchedSelector`:

```go
func TestFilterPlansErrorsOnUnmatchedSelector(t *testing.T) {
	plans := []Plan{{ArchiveName: "a.txt"}}

	_, err := FilterPlans(plans, []string{"a.txt", "missing.txt"})
	if err == nil {
		t.Fatal("FilterPlans() with unmatched selector succeeded")
	}
}
```

becomes:

```go
func TestFilterPlansErrorsOnUnmatchedSelector(t *testing.T) {
	plans := []Plan{{ArchiveName: "a.txt"}}

	_, err := FilterPlans(plans, []string{"a.txt", "missing.txt"})
	if err == nil {
		t.Fatal("FilterPlans() with unmatched selector succeeded")
	}
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("FilterPlans() error = %v, want ErrNoMatch", err)
	}
}
```

Add `"errors"` to `pkg/unpack/select_test.go`'s import block (it currently only imports
`"testing"`).

In `pkg/unpack/zip_test.go`, in `TestScanZIPRejectsEncryptedEntry`:

```go
func TestScanZIPRejectsEncryptedEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "encrypted.zip")
	writeZIPFixture(t, path, []zipFixture{{Name: "secret.txt", Body: "secret"}})
	patchZIPEncrypted(t, path)

	if _, err := scanZIP(path, false); err == nil {
		t.Fatal("scanZIP() accepted an encrypted entry")
	}
}
```

becomes:

```go
func TestScanZIPRejectsEncryptedEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "encrypted.zip")
	writeZIPFixture(t, path, []zipFixture{{Name: "secret.txt", Body: "secret"}})
	patchZIPEncrypted(t, path)

	_, err := scanZIP(path, false)
	if err == nil {
		t.Fatal("scanZIP() accepted an encrypted entry")
	}
	if !errors.Is(err, ErrUnsafeEntry) {
		t.Fatalf("scanZIP() error = %v, want ErrUnsafeEntry", err)
	}
}
```

(`pkg/unpack/zip_test.go` already imports `"errors"`.)

In `pkg/unpack/tar_test.go`, in `TestScanTARRejectsHardLink`:

```go
func TestScanTARRejectsHardLink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "link.tar")
	writeTARFixture(t, path, false, []tarFixture{{
		Name:     "linked",
		Mode:     0o777,
		Typeflag: tar.TypeLink,
		Linkname: "target",
	}})

	if _, err := scanTAR(path, FormatTAR, false); err == nil {
		t.Fatal("scanTAR() accepted a hard link entry")
	}
}
```

becomes:

```go
func TestScanTARRejectsHardLink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "link.tar")
	writeTARFixture(t, path, false, []tarFixture{{
		Name:     "linked",
		Mode:     0o777,
		Typeflag: tar.TypeLink,
		Linkname: "target",
	}})

	_, err := scanTAR(path, FormatTAR, false)
	if err == nil {
		t.Fatal("scanTAR() accepted a hard link entry")
	}
	if !errors.Is(err, ErrUnsafeEntry) {
		t.Fatalf("scanTAR() error = %v, want ErrUnsafeEntry", err)
	}
}
```

Add `"errors"` to `pkg/unpack/tar_test.go`'s import block (it currently imports `"archive/tar"`,
`"io/fs"`, `"os"`, `"path/filepath"`, `"testing"`).

In `pkg/unpack/path_test.go`, add an `errors.Is` check to each of these four existing tests
(`"errors"` is already imported):

`TestPlanEntriesRejectsTraversal`:

```go
		_, _, err := PlanEntries(
			[]Entry{{Name: name, Kind: EntryFile}}, "bundle.zip", root, root, FormatZIP,
		)
		if err == nil {
			t.Fatalf("unsafe name %q succeeded", name)
		}
```

becomes:

```go
		_, _, err := PlanEntries(
			[]Entry{{Name: name, Kind: EntryFile}}, "bundle.zip", root, root, FormatZIP,
		)
		if err == nil {
			t.Fatalf("unsafe name %q succeeded", name)
		}
		if !errors.Is(err, ErrUnsafeEntry) {
			t.Fatalf("PlanEntries(%q) error = %v, want ErrUnsafeEntry", name, err)
		}
```

`TestPlanEntriesRejectsSymlinkTargetEscape`, same transformation on its `if err == nil {
t.Fatalf("unsafe symlink target %q succeeded", target) }` block — add the matching
`errors.Is(err, ErrUnsafeEntry)` check referencing `target` in the failure message.

`TestPlanEntriesRejectsSymlinkAncestor`:

```go
	if err == nil {
		t.Fatal("entry nested under a symlink succeeded")
	}
```

becomes:

```go
	if err == nil {
		t.Fatal("entry nested under a symlink succeeded")
	}
	if !errors.Is(err, ErrUnsafeEntry) {
		t.Fatalf("PlanEntries() error = %v, want ErrUnsafeEntry", err)
	}
```

`TestPlanEntriesRejectsExistingSymlinkEscape`, same transformation on its `if err == nil { t.Fatal
("entry through pre-existing symlink succeeded") }` block.

- [ ] **Step 13: Build and test `pkg/unpack` in isolation**

```bash
gofmt -l pkg/unpack
go vet ./pkg/unpack/...
go test ./pkg/unpack/... -v
```

Expected: `gofmt -l` prints nothing; `go vet` and `go test` both pass with no failures. At this
point `go build ./...` and `go test ./...` at the repo root will still fail, because `main.go`
still imports the now-deleted `unpack/internal/unpack` package — that's expected and gets fixed in
Task 2.

- [ ] **Step 14: Commit**

```bash
git add pkg/unpack
git commit -m "feat: add pkg/unpack public library API"
```

---

## Task 2: Cut over the CLI to `pkg/unpack` and delete `internal/unpack`

**Files:**
- Delete: `internal/unpack/` (entire directory: `run.go`, `run_test.go`, `version.go`)
- Modify: `main.go`
- Create: `cli.go`, `cli_test.go`, `version.go` (moved), `scripts/build.sh`, `AGENTS.md`

**Interfaces:**
- Consumes: everything produced in Task 1 (`unpack.Options`, `unpack.Result`, `unpack.Extract`,
  `unpack.IsRemoteURL`, `unpack.Download`)
- Produces: `func run(args []string, stdout, stderr io.Writer, workingDir string) int` (called by
  `main()`; also the entry point `cli_test.go` calls directly, same as `run_test.go` called `Run`
  today)

- [ ] **Step 1: Move `version.go` as-is**

```bash
git mv internal/unpack/version.go version.go
sed -i 's/^package unpack$/package main/' version.go
```

- [ ] **Step 2: Create `cli.go` from `run.go`, switching to `pkg/unpack`**

```bash
git mv internal/unpack/run.go cli.go
```

Read the current file at `internal/unpack/run.go` (or `cli.go` after the move) and rewrite it as
follows — the flag definitions, usage text, and `validateOptionSyntax` are unchanged; only the
package declaration, the exported `Run` → unexported `run` rename, and the body of what used to be
`processArchive` change:

```go
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"unpack/pkg/unpack"
)

func run(args []string, stdout io.Writer, stderr io.Writer, workingDir string) int {
	var (
		flags         *flag.FlagSet
		chinese       bool
		outputDir     string
		overwrite     bool
		showVersion   bool
		extractionDir string
		archivePath   string
		selectors     []string
	)

	flags = flag.NewFlagSet("unpack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&chinese, "cn", false, "decode legacy Chinese filenames as GBK")
	flags.StringVar(&outputDir, "directory", "", "extract into DIR")
	flags.StringVar(&outputDir, "d", "", "extract into DIR (shorthand for --directory)")
	flags.BoolVar(&overwrite, "overwrite", false, "overwrite existing files and directories")
	flags.BoolVar(&showVersion, "version", false, "print version information and exit")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: unpack [--cn] [--directory DIR] [--overwrite] [--version] ARCHIVE [FILE...]")
		fmt.Fprintln(stderr, "  --cn                 decode legacy Chinese filenames as GBK")
		fmt.Fprintln(stderr, "  --directory, -d DIR  extract into DIR")
		fmt.Fprintln(stderr, "  --overwrite          overwrite existing files and directories")
		fmt.Fprintln(stderr, "  --version            print version information and exit")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "  FILE...              extract only entries matching these selectors")
		fmt.Fprintln(stderr, "                       (exact paths, directory prefixes, or globs;")
		fmt.Fprintln(stderr, "                       globs may cross '/', e.g. *.md)")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "  ARCHIVE may be a local path or an http(s):// URL, downloaded to a")
		fmt.Fprintln(stderr, "  temporary file before extraction.")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Project: https://github.com/d2jvkpn/unpack")
	}

	if err := validateOptionSyntax(args); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if showVersion {
		var (
			revision   string
			commitTime string
			modified   bool
		)
		revision, commitTime, modified = vcsInfo()
		fmt.Fprintf(stdout, "version:     %s\n", version)
		fmt.Fprintf(stdout, "commit:      %s\n", revision)
		fmt.Fprintf(stdout, "commit_time: %s\n", commitTime)
		fmt.Fprintf(stdout, "modified:    %t\n", modified)
		fmt.Fprintf(stdout, "build_time:  %s\n", buildTime)
		return 0
	}
	if flags.NArg() == 0 {
		flags.Usage()
		return 2
	}
	extractionDir = outputDir
	if extractionDir != "" && !filepath.IsAbs(extractionDir) {
		extractionDir = filepath.Join(workingDir, extractionDir)
	}

	archivePath = flags.Args()[0]
	if unpack.IsRemoteURL(archivePath) {
		fmt.Fprintf(stdout, "Downloading: %s\n", archivePath)
		localPath, cleanup, err := unpack.Download(archivePath)
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		defer cleanup()
		archivePath = localPath
	}
	selectors = flags.Args()[1:]

	fmt.Fprintf(stdout, "Extracting: %s\n", archivePath)
	result, err := unpack.Extract(archivePath, unpack.Options{
		Directory:  extractionDir,
		WorkingDir: workingDir,
		Chinese:    chinese,
		Overwrite:  overwrite,
		Selectors:  selectors,
	})
	fmt.Fprintf(stdout, "Output directory: %s\n", result.Directory)
	for _, relativeName := range result.Skipped {
		fmt.Fprintf(stdout, "Skipping existing file: %s\n", relativeName)
	}
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

func validateOptionSyntax(args []string) error {
	var skipOperand bool

	for _, arg := range args {
		if skipOperand {
			skipOperand = false
			continue
		}
		if arg == "--" {
			return nil
		}
		if arg == "--directory" || arg == "-d" {
			skipOperand = true
			continue
		}
		if arg == "-h" || arg == "--help" {
			return nil
		}
		if !strings.HasPrefix(arg, "-") {
			return nil
		}
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			return fmt.Errorf("flag provided but not defined: %s", arg)
		}
	}
	return nil
}
```

Note the `Output directory:` line moves to *after* the `unpack.Extract` call (it now reads
`result.Directory`, which `Extract` only knows once planning has happened), and the whole
`processArchive`/`detectFormat`/`scanArchive`/`planEntries`/`filterPlans`/`extractArchive` chain
collapses into the single `unpack.Extract` call — this is expected to shrink the file, not grow
it. Preserve the print order exactly (`Extracting:` immediately after the URL-download branch,
before calling `Extract`, matching `internal/unpack/run.go`'s ordering of `Extracting:` printed
before the plan/write phase runs).

- [ ] **Step 3: Rewrite `main.go`**

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	var (
		workingDir string
		err        error
	)

	workingDir, err = os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: get current working directory: %v\n", err)
		os.Exit(1)
	}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, workingDir))
}
```

- [ ] **Step 4: Create `cli_test.go` with the CLI-only tests from `run_test.go`**

Move only these test functions from `internal/unpack/run_test.go` into a new root-level
`cli_test.go` (`package main`), renaming every `Run(...)` call to `run(...)`:

- `TestRunShowsHelpAndParsesSupportedFlags` — but drop its `"cn and output directory"` subtest
  (that GBK+extraction behavior is now covered by `pkg/unpack`'s
  `TestExtractUsesSmartDefaultDestinations`-style tests and doesn't need re-verifying through the
  CLI); keep only the `"help"` subtest.
- `TestRunRequiresAtLeastOneArchive`
- `TestRunRejectsShortOutputAlias`
- `TestRunRejectsSingleDashLongOptionsWithoutWriting`
- `TestRunAcceptsDashPrefixedOutputDirectoryOperand`
- `TestRunStopsOptionParsingAtFirstArchive`
- `TestRunDownloadsArchiveFromURL`
- `TestRunReportsErrorForMissingArchive`
- `TestRunTreatsTrailingArchivePathAsSelectorNotSecondArchive`

These tests build ZIP fixtures directly with `archive/zip` (not through `pkg/unpack`), so
`cli_test.go` needs its own small fixture helper — it does not need TAR fixtures, since none of
the tests kept here exercise TAR. Add this helper directly in `cli_test.go` alongside the moved
tests, along with `assertFileContents` and `assertPathDoesNotExist` (also moved from
`run_test.go`, unchanged):

```go
package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type zipFixture struct {
	Name string
	Body string
	Mode fs.FileMode
}

func writeZIPFixture(t *testing.T, path string, entries []zipFixture) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.Name, Method: zip.Deflate}
		if entry.Mode != 0 {
			header.SetMode(entry.Mode)
		}
		entryWriter, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entryWriter, entry.Body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertFileContents(t *testing.T, path string, want string) {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != want {
		t.Fatalf("contents of %q = %q, %v; want %q", path, contents, err, want)
	}
}

func assertPathDoesNotExist(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("path %q exists or returned unexpected error: %v", path, err)
	}
}
```

Then append the nine moved test functions below, each with its `Run(...)` calls changed to
`run(...)` and, since `zipFixture` here has no `NonUTF8`/`Flags` fields (nothing kept in this file
needs them), no other changes — every kept test only used `Name`, `Body`, and occasionally `Mode`.

- [ ] **Step 5: Fix the version ldflags path in `scripts/build.sh`**

```bash
grep -n "unpack/internal/unpack.buildTime" scripts/build.sh
```

Expected: one match, on the `LDFLAGS=` line. In `scripts/build.sh`:

```sh
LDFLAGS="-X unpack/internal/unpack.buildTime=$BUILD_TIME"
```

becomes:

```sh
LDFLAGS="-X unpack.buildTime=$BUILD_TIME"
```

(The root `package main` compiled as the module's main package has import path `unpack` — the
module path itself — since it has no subdirectory, so `-X unpack.buildTime=...` is the correct
target now that `buildTime` lives in the root `version.go`.)

- [ ] **Step 6: Delete the rest of `internal/unpack`**

`run.go` and `version.go` already moved out via `git mv` in Steps 1-2. Everything left under
`internal/unpack` — `run_test.go` — has had all of its content redistributed: the CLI-only tests
went to `cli_test.go` in Step 4, the library-level tests went to `pkg/unpack/extract_test.go` in
Task 1 Step 11, and its two assertion helpers (`assertFileContents`, `assertPathDoesNotExist`)
were copied into `cli_test.go` in Step 4 too. Nothing in it needs to survive independently:

```bash
git rm -r internal/unpack
git status
```

Confirm `git status` shows no remaining files under `internal/` and that the `internal/`
directory itself is gone (`git rm -r` removes now-empty parent directories automatically).

- [ ] **Step 7: Update `AGENTS.md`'s project structure section**

In `AGENTS.md`, replace:

```markdown
This repository contains a small Go command-line application for safely extracting ZIP, TAR,
TAR.GZ, and TGZ archives. The root `main.go` is a thin entry point that only resolves the working
directory and delegates to the `internal/unpack` package. Within `internal/unpack`, command
orchestration and flag parsing live in `run.go`; archive format detection is in `archive.go`; ZIP
and TAR handling are separated into `zip.go` and `tar.go`; path validation and extraction planning
live in `path.go`; and filename decoding helpers are in `names.go`. Tests are colocated with their
implementation as `*_test.go`. `unzip_cn.py` and `test_unzip_cn.py` provide the legacy Python
implementation and its tests. Design records are stored under `docs/records/superpowers/`.
```

with:

```markdown
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
```

- [ ] **Step 8: Full-repo build and test**

```bash
gofmt -l .
go vet ./...
go test ./...
go build -o /tmp/unpack .
```

Expected: `gofmt -l .` prints nothing; `go vet`, `go test ./...`, and the build all succeed.

- [ ] **Step 9: Manual smoke test**

```bash
/tmp/unpack --help
cd /tmp && rm -rf unpack-smoke && mkdir unpack-smoke && cd unpack-smoke \
  && echo hi > a.txt && zip -q test.zip a.txt \
  && /tmp/unpack -d out test.zip && cat out/a.txt
```

Expected: `--help` prints the usage text with `--directory, -d DIR`; the zip extracts `a.txt` into
`out/` with contents `hi`.

- [ ] **Step 10: Verify the release build's version metadata still resolves**

```bash
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  go build -ldflags "-X unpack.buildTime=$BUILD_TIME" -o /tmp/unpack-versioned .
/tmp/unpack-versioned --version
```

Expected: the `build_time:` line prints the injected timestamp rather than `unknown`, confirming
the `scripts/build.sh` ldflags fix in Step 5 targets the right symbol.

- [ ] **Step 11: Commit**

```bash
git add -A
git commit -m "refactor: move CLI glue to package main, delete internal/unpack"
```

---

## Self-review notes

- **Spec coverage:** directory layout (Task 1 Step 1, Task 2 Steps 1-2/6), `Format`/`EntryKind`/
  `Entry`/`Plan` types (Task 1 Step 2), high-level `Extract`/`Options`/`Result` (Task 1 Step 10),
  low-level primitives (Task 1 Step 2 renames), sentinel errors (Task 1 Steps 4-9), `Result`
  partially populated on error (Task 1 Step 10's `WritePlans` handling, tested in Task 1 Step 11's
  `TestExtractReportsSkippedFilesInResultOnLaterFailure`), CLI composition (Task 2 Step 2), test
  migration split (Task 1 Step 11, Task 2 Step 4), `scripts/build.sh` and `AGENTS.md` sync (Task 2
  Steps 5/7) — all covered.
- **Placeholder scan:** no TBD/TODO; every step has literal commands or full code.
- **Type consistency:** `Options`/`Result` field names match between `extract.go` (Task 1 Step
  10) and `cli.go` (Task 2 Step 2); `Format`/`Entry`/`Plan`/`EntryKind` names match between the
  rename script (Task 1 Step 2) and every subsequent snippet that references them.
