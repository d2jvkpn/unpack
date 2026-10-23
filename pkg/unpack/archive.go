// Package unpack safely extracts ZIP, TAR, compressed TAR variants, and standalone Bzip2 files: it validates every entry's
// destination before writing anything to disk, rejecting path traversal, symlink escapes, and
// unsupported entry types. Extract is the high-level entry point; DetectFormat, Scan,
// PlanEntries, FilterPlans, and WritePlans are lower-level primitives for callers that want to
// assemble their own pipeline.
package unpack

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Format identifies which archive container a path was detected as.
type Format uint8

// Supported archive formats.
const (
	FormatZIP Format = iota + 1
	FormatTAR
	FormatTarGzip
	FormatTarXz
	FormatTarBzip2
	FormatBzip2
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

// DetectFormat identifies an archive's format. It first sniffs the file's leading bytes (ZIP and
// gzip magic numbers, TAR header), so a misnamed or extensionless archive is still recognized, and
// falls back to the filename suffix when the content isn't recognized or can't be read. It
// returns ErrUnsupportedFormat, wrapped with the rejected filename, when neither identifies a
// supported format.
func DetectFormat(path string) (Format, error) {
	var (
		format Format
		lower  string
	)

	format = sniffFormat(path)
	if format != 0 {
		return format, nil
	}

	lower = strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return FormatTarGzip, nil
	case strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".txz"):
		return FormatTarXz, nil
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"),
		strings.HasSuffix(lower, ".tbz"), strings.HasSuffix(lower, ".tar.bz"):
		return FormatTarBzip2, nil
	case strings.HasSuffix(lower, ".bz"), strings.HasSuffix(lower, ".bzip2"):
		return FormatBzip2, nil
	case strings.HasSuffix(lower, ".tar"):
		return FormatTAR, nil
	case strings.HasSuffix(lower, ".zip"):
		return FormatZIP, nil
	default:
		return 0, fmt.Errorf("%w: %s", ErrUnsupportedFormat, filepath.Base(path))
	}
}

// ParseFormat converts a user-supplied format name into a Format, case-insensitively. It returns
// ErrUnsupportedFormat, wrapped, for anything else.
func ParseFormat(name string) (Format, error) {
	switch strings.ToLower(name) {
	case "zip":
		return FormatZIP, nil
	case "tar":
		return FormatTAR, nil
	case "tar.gz", "tgz":
		return FormatTarGzip, nil
	case "tar.xz", "txz":
		return FormatTarXz, nil
	case "tar.bz2", "tbz2", "tar.bz", "tbz":
		return FormatTarBzip2, nil
	case "bz", "bzip2":
		return FormatBzip2, nil
	default:
		return 0, fmt.Errorf("%w: %q (want zip, tar, tar.gz, tar.xz, tar.bz, or bz)", ErrUnsupportedFormat, name)
	}
}

const tarBlockSize = 512

// sniffFormat identifies the format from file content alone, returning 0 when the content isn't
// recognized or the file can't be read.
func sniffFormat(path string) Format {
	var (
		file   *os.File
		head   []byte
		n      int
		format Format
		inner  io.Reader
		block  []byte
		err    error
	)

	file, err = os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()

	head = make([]byte, tarBlockSize)
	n, _ = io.ReadFull(file, head)
	head = head[:n]

	switch {
	case bytes.HasPrefix(head, []byte("PK\x03\x04")), bytes.HasPrefix(head, []byte("PK\x05\x06")):
		return FormatZIP
	case bytes.HasPrefix(head, []byte{0x1f, 0x8b}):
		format = FormatTarGzip
	case bytes.HasPrefix(head, []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}):
		format = FormatTarXz
	case bytes.HasPrefix(head, []byte("BZh")):
		format = FormatBzip2
	case isTARHeader(head):
		return FormatTAR
	default:
		return 0
	}

	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return 0
	}
	inner, err = decompress(format, file)
	if err != nil {
		return 0
	}
	if closer, ok := inner.(io.Closer); ok {
		defer closer.Close()
	}
	block = make([]byte, tarBlockSize)
	n, _ = io.ReadFull(inner, block)
	if isTARHeader(block[:n]) {
		if format == FormatBzip2 {
			return FormatTarBzip2
		}
		return format
	}
	if format == FormatBzip2 {
		_, err = io.Copy(io.Discard, inner)
		if err == nil {
			return FormatBzip2
		}
	}
	return 0
}

// isTARHeader reports whether block looks like a TAR header: either the POSIX "ustar" magic, or
// (for V7 archives without it) a valid, non-blank header checksum.
func isTARHeader(block []byte) bool {
	var (
		sum    int
		stored int64
		field  string
		err    error
	)

	if len(block) < tarBlockSize {
		return false
	}
	if bytes.HasPrefix(block[257:], []byte("ustar")) {
		return true
	}
	for i, b := range block {
		if i >= 148 && i < 156 {
			b = ' '
		}
		sum += int(b)
	}
	if sum == 8*' ' {
		return false
	}
	field = strings.Trim(string(block[148:156]), " \x00")
	stored, err = strconv.ParseInt(field, 8, 64)
	return err == nil && stored == int64(sum)
}

func archiveBaseName(path string, format Format) string {
	var (
		name     string
		lower    string
		suffixes []string
	)

	name = filepath.Base(path)
	lower = strings.ToLower(name)
	switch format {
	case FormatZIP:
		suffixes = []string{".zip"}
	case FormatTAR:
		suffixes = []string{".tar"}
	case FormatTarGzip:
		suffixes = []string{".tar.gz", ".tgz"}
	case FormatTarXz:
		suffixes = []string{".tar.xz", ".txz"}
	case FormatTarBzip2:
		suffixes = []string{".tar.bz2", ".tbz2", ".tbz", ".tar.bz"}
	case FormatBzip2:
		suffixes = []string{".bz", ".bzip2"}
	}
	for _, suffix := range suffixes {
		if strings.HasSuffix(lower, suffix) {
			return name[:len(name)-len(suffix)]
		}
	}
	return name
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
	case FormatTAR, FormatTarGzip, FormatTarXz, FormatTarBzip2:
		entries, err = scanTAR(path, format, chinese)
	case FormatBzip2:
		entries, err = scanBzip2(path)
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
	case FormatTAR, FormatTarGzip, FormatTarXz, FormatTarBzip2:
		skipped, err = extractTAR(path, format, plans, overwrite)
	case FormatBzip2:
		skipped, err = extractBzip2(path, plans, overwrite)
	default:
		err = fmt.Errorf("unsupported archive format %d", format)
	}
	if err != nil {
		return skipped, fmt.Errorf("extract archive %q: %w", path, err)
	}
	return skipped, nil
}
