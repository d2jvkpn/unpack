package unpack

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

type archiveFormat uint8

const (
	formatZIP archiveFormat = iota + 1
	formatTAR
	formatTarGzip
)

type entryKind uint8

const (
	entryFile entryKind = iota + 1
	entryDirectory
)

type archiveEntry struct {
	Name        string
	Kind        entryKind
	Mode        fs.FileMode
	SourceIndex int
}

func detectFormat(path string) (archiveFormat, error) {
	var lower string

	lower = strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return formatTarGzip, nil
	case strings.HasSuffix(lower, ".tar"):
		return formatTAR, nil
	case strings.HasSuffix(lower, ".zip"):
		return formatZIP, nil
	default:
		return 0, fmt.Errorf("unsupported archive format: %s", filepath.Base(path))
	}
}

func archiveBaseName(path string, format archiveFormat) string {
	var (
		name   string
		lower  string
		suffix string
	)

	name = filepath.Base(path)
	lower = strings.ToLower(name)
	suffix = map[archiveFormat]string{
		formatZIP: ".zip", formatTAR: ".tar", formatTarGzip: ".tgz",
	}[format]
	if format == formatTarGzip && strings.HasSuffix(lower, ".tar.gz") {
		suffix = ".tar.gz"
	}
	return name[:len(name)-len(suffix)]
}

func scanArchive(path string, format archiveFormat, chinese bool) ([]archiveEntry, error) {
	var (
		entries []archiveEntry
		err     error
	)
	switch format {
	case formatZIP:
		entries, err = scanZIP(path, chinese)
	case formatTAR, formatTarGzip:
		entries, err = scanTAR(path, format, chinese)
	default:
		err = fmt.Errorf("unsupported archive format %d", format)
	}
	if err != nil {
		return nil, fmt.Errorf("scan archive %q: %w", path, err)
	}
	return entries, nil
}

func extractArchive(
	path string,
	format archiveFormat,
	plans []plannedEntry,
	overwrite bool,
) ([]string, error) {
	var (
		skipped []string
		err     error
	)
	switch format {
	case formatZIP:
		skipped, err = extractZIP(path, plans, overwrite)
	case formatTAR, formatTarGzip:
		skipped, err = extractTAR(path, format, plans, overwrite)
	default:
		err = fmt.Errorf("unsupported archive format %d", format)
	}
	if err != nil {
		return skipped, fmt.Errorf("extract archive %q: %w", path, err)
	}
	return skipped, nil
}
