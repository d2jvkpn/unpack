// Package unpack safely extracts ZIP, TAR, TAR.GZ, and TGZ archives: it validates every entry's
// destination before writing anything to disk, rejecting path traversal, symlink escapes, and
// unsupported entry types. Extract is the high-level entry point; DetectFormat, Scan,
// PlanEntries, FilterPlans, and WritePlans are lower-level primitives for callers that want to
// assemble their own pipeline.
package unpack

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// Format identifies which archive container a path was detected as.
type Format uint8

// Supported archive formats.
const (
	FormatZIP Format = iota + 1
	FormatTAR
	FormatTarGzip
)

// EntryKind identifies what an archive entry represents on disk.
type EntryKind uint8

// Supported entry kinds.
const (
	EntryFile EntryKind = iota + 1
	EntryDirectory
	EntrySymlink
)

// Entry is a single archive member discovered by Scan.
type Entry struct {
	Name        string
	Kind        EntryKind
	Mode        fs.FileMode
	LinkTarget  string
	SourceIndex int
}

// DetectFormat identifies an archive's format from its filename suffix. It returns
// ErrUnsupportedFormat, wrapped with the rejected filename, when the suffix isn't recognized.
func DetectFormat(path string) (Format, error) {
	var lower string

	lower = strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return FormatTarGzip, nil
	case strings.HasSuffix(lower, ".tar"):
		return FormatTAR, nil
	case strings.HasSuffix(lower, ".zip"):
		return FormatZIP, nil
	default:
		return 0, fmt.Errorf("%w: %s", ErrUnsupportedFormat, filepath.Base(path))
	}
}

func archiveBaseName(path string, format Format) string {
	var (
		name   string
		lower  string
		suffix string
	)

	name = filepath.Base(path)
	lower = strings.ToLower(name)
	suffix = map[Format]string{
		FormatZIP: ".zip", FormatTAR: ".tar", FormatTarGzip: ".tgz",
	}[format]
	if format == FormatTarGzip && strings.HasSuffix(lower, ".tar.gz") {
		suffix = ".tar.gz"
	}
	return name[:len(name)-len(suffix)]
}

// Scan reads every entry in the archive at path without writing anything to disk. It returns
// ErrUnsafeEntry, wrapped with details, for entry kinds this package can't safely represent
// (hard links, devices, FIFOs, encrypted ZIP entries, directory-named symlinks) and for
// undecodable legacy Chinese filenames.
func Scan(path string, format Format, chinese bool) ([]Entry, error) {
	var (
		entries []Entry
		err     error
	)
	switch format {
	case FormatZIP:
		entries, err = scanZIP(path, chinese)
	case FormatTAR, FormatTarGzip:
		entries, err = scanTAR(path, format, chinese)
	default:
		err = fmt.Errorf("unsupported archive format %d", format)
	}
	if err != nil {
		return nil, fmt.Errorf("scan archive %q: %w", path, err)
	}
	return entries, nil
}

// WritePlans writes the given plans, as produced by PlanEntries, to disk. It returns the
// archive-relative names of entries that already existed and were preserved (overwrite is
// false), and does so even when it also returns a non-nil error: entries written or skipped
// before a later failure are still reported.
func WritePlans(
	path string,
	format Format,
	plans []Plan,
	overwrite bool,
) ([]string, error) {
	var (
		skipped []string
		err     error
	)
	switch format {
	case FormatZIP:
		skipped, err = extractZIP(path, plans, overwrite)
	case FormatTAR, FormatTarGzip:
		skipped, err = extractTAR(path, format, plans, overwrite)
	default:
		err = fmt.Errorf("unsupported archive format %d", format)
	}
	if err != nil {
		return skipped, fmt.Errorf("extract archive %q: %w", path, err)
	}
	return skipped, nil
}
