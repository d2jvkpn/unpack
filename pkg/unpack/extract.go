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
