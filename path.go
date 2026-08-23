package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type plannedEntry struct {
	Entry        archiveEntry
	RelativeName string
	Destination  string
}

func planEntries(
	entries []archiveEntry,
	archivePath string,
	outputDir string,
	workingDir string,
	format archiveFormat,
) ([]plannedEntry, string, error) {
	var (
		names              []string
		rootMarkers        []bool
		topLevel           map[string]struct{}
		base               string
		stripTopLevel      string
		syntheticDefault   bool
		absBase            string
		resolvedBase       string
		absWorkingDir      string
		resolvedWorkingDir string
		plans              []plannedEntry
		err                error
	)

	names = make([]string, len(entries))
	rootMarkers = make([]bool, len(entries))
	topLevel = make(map[string]struct{})
	for i, entry := range entries {
		switch entry.Kind {
		case entryFile, entryDirectory:
		default:
			return nil, "", fmt.Errorf("unsafe archive entry %q: unsupported entry kind %d", entry.Name, entry.Kind)
		}
		name, err := normalizeArchivePath(entry.Name)
		if err != nil {
			return nil, "", fmt.Errorf("unsafe archive entry %q: %w", entry.Name, err)
		}
		names[i] = name
		if name == "." {
			if entry.Kind != entryDirectory {
				return nil, "", fmt.Errorf("unsafe archive entry %q: root marker is not a directory", entry.Name)
			}
			rootMarkers[i] = true
			continue
		}
		topLevel[strings.SplitN(name, "/", 2)[0]] = struct{}{}
	}
	if err := rejectFileTopLevelAncestors(names, entries); err != nil {
		return nil, "", err
	}

	base = outputDir
	if outputDir == "" {
		if len(topLevel) > 1 {
			stem := archiveBaseName(archivePath, format)
			if !filepath.IsLocal(stem) || filepath.Clean(stem) == "." {
				return nil, "", fmt.Errorf("unsafe archive base name %q", stem)
			}
			base = filepath.Join(workingDir, stem)
			syntheticDefault = true
		} else {
			base = workingDir
		}
	} else if len(topLevel) == 1 {
		for top := range topLevel {
			if isTopLevelDirectory(top, names, entries) {
				stripTopLevel = top
			}
		}
	}

	absBase, err = filepath.Abs(base)
	if err != nil {
		return nil, "", fmt.Errorf("resolve extraction base %q: %w", base, err)
	}
	resolvedBase, err = resolveExistingPath(absBase)
	if err != nil {
		return nil, "", fmt.Errorf("resolve extraction base %q: %w", absBase, err)
	}
	if syntheticDefault {
		absWorkingDir, err = filepath.Abs(workingDir)
		if err != nil {
			return nil, "", fmt.Errorf("resolve working directory %q: %w", workingDir, err)
		}
		resolvedWorkingDir, err = resolveExistingPath(absWorkingDir)
		if err != nil {
			return nil, "", fmt.Errorf("resolve working directory %q: %w", absWorkingDir, err)
		}
		if err := requireStrictlyWithin(resolvedWorkingDir, resolvedBase); err != nil {
			return nil, "", fmt.Errorf("unsafe archive base %q: %w", base, err)
		}
	}

	plans = make([]plannedEntry, 0, len(entries))
	for i, entry := range entries {
		if rootMarkers[i] {
			continue
		}
		relativeName := names[i]
		if stripTopLevel != "" {
			if relativeName == stripTopLevel {
				relativeName = ""
			} else {
				relativeName = strings.TrimPrefix(relativeName, stripTopLevel+"/")
			}
		}
		destination := filepath.Join(absBase, filepath.FromSlash(relativeName))
		resolvedDestination, err := resolveExistingPath(destination)
		if err != nil {
			return nil, "", fmt.Errorf("unsafe archive entry %q: resolve destination: %w", entry.Name, err)
		}
		if err := requireWithin(resolvedBase, resolvedDestination); err != nil {
			return nil, "", fmt.Errorf("unsafe archive entry %q: %w", entry.Name, err)
		}
		plans = append(plans, plannedEntry{
			Entry:        entry,
			RelativeName: filepath.FromSlash(relativeName),
			Destination:  destination,
		})
	}

	return plans, absBase, nil
}

func normalizeArchivePath(name string) (string, error) {
	var cleaned string

	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || path.IsAbs(name) || isDriveRootPath(name) {
		return "", errors.New("path is empty or absolute")
	}
	cleaned = path.Clean(name)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errors.New("path traverses outside the extraction directory")
	}
	return cleaned, nil
}

func isDriveRootPath(name string) bool {
	return len(name) >= 3 && name[1] == ':' && name[2] == '/'
}

func rejectFileTopLevelAncestors(names []string, entries []archiveEntry) error {
	var (
		topLevelFiles    map[string]int
		firstDescendants map[string]int
	)

	topLevelFiles = make(map[string]int)
	for i, name := range names {
		if entries[i].Kind != entryFile || strings.Contains(name, "/") {
			continue
		}
		if _, exists := topLevelFiles[name]; !exists {
			topLevelFiles[name] = i
		}
	}
	firstDescendants = make(map[string]int)
	for j, descendant := range names {
		topLevel, _, hasDescendant := strings.Cut(descendant, "/")
		if !hasDescendant {
			continue
		}
		if _, exists := topLevelFiles[topLevel]; !exists {
			continue
		}
		if _, exists := firstDescendants[topLevel]; !exists {
			firstDescendants[topLevel] = j
		}
	}
	for i, name := range names {
		firstFile, isTopLevelFile := topLevelFiles[name]
		if !isTopLevelFile || firstFile != i {
			continue
		}
		if j, hasDescendant := firstDescendants[name]; hasDescendant {
			return fmt.Errorf("unsafe archive entries %q and %q: file entry is an ancestor", entries[i].Name, entries[j].Name)
		}
	}
	return nil
}

func isTopLevelDirectory(top string, names []string, entries []archiveEntry) bool {
	for i, name := range names {
		if name == top && entries[i].Kind == entryDirectory {
			return true
		}
		if strings.HasPrefix(name, top+"/") {
			return true
		}
	}
	return false
}

func resolveExistingPath(name string) (string, error) {
	var (
		missing  []string
		existing string
		resolved string
		err      error
	)

	name = filepath.Clean(name)
	existing = name
	for {
		_, err = os.Lstat(existing)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", err
		}
		missing = append(missing, filepath.Base(existing))
		existing = parent
	}

	resolved, err = filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	for i := len(missing) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, missing[i])
	}
	return resolved, nil
}

func requireWithin(base string, destination string) error {
	var (
		relative string
		err      error
	)

	relative, err = filepath.Rel(base, destination)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("destination escapes extraction base")
	}
	return nil
}

func requireStrictlyWithin(base string, destination string) error {
	var (
		relative string
		err      error
	)

	relative, err = filepath.Rel(base, destination)
	if err != nil {
		return err
	}
	if relative == "." {
		return fmt.Errorf("destination is not beneath extraction root")
	}
	return requireWithin(base, destination)
}

type directoryFinalizer struct {
	created    map[string]struct{}
	finalModes map[string]fs.FileMode
	overwrite  bool
}

func newDirectoryFinalizer(overwrite bool) *directoryFinalizer {
	return &directoryFinalizer{
		created:    make(map[string]struct{}),
		finalModes: make(map[string]fs.FileMode),
		overwrite:  overwrite,
	}
}

func (d *directoryFinalizer) ensure(name string) error {
	var (
		missing []string
		current string
	)

	name = filepath.Clean(name)
	missing = make([]string, 0)
	current = name
	for {
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() {
				if !d.overwrite {
					return fmt.Errorf("path %q is not a directory", current)
				}
				if err := os.Remove(current); err != nil {
					return fmt.Errorf("remove existing file %q: %w", current, err)
				}
				missing = append(missing, current)
				current = filepath.Dir(current)
				continue
			}
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return err
		}
		missing = append(missing, current)
		current = parent
	}

	for i := len(missing) - 1; i >= 0; i-- {
		directory := missing[i]
		if err := os.Mkdir(directory, 0o700); err != nil {
			if errors.Is(err, fs.ErrExist) {
				info, statErr := os.Stat(directory)
				if statErr == nil && info.IsDir() {
					continue
				}
			}
			return fmt.Errorf("create directory %q: %w", directory, err)
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return fmt.Errorf("make directory %q owner-accessible: %w", directory, err)
		}
		d.created[directory] = struct{}{}
	}
	return nil
}

func (d *directoryFinalizer) recordMode(name string, mode fs.FileMode) {
	d.finalModes[filepath.Clean(name)] = directoryMode(mode)
}

func (d *directoryFinalizer) finalize() error {
	var directories []string

	directories = make([]string, 0, len(d.created))
	for directory := range d.created {
		directories = append(directories, directory)
	}
	sort.Slice(directories, func(i, j int) bool {
		var (
			leftDepth  int
			rightDepth int
		)

		leftDepth = strings.Count(filepath.Clean(directories[i]), string(filepath.Separator))
		rightDepth = strings.Count(filepath.Clean(directories[j]), string(filepath.Separator))
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		return directories[i] > directories[j]
	})
	for _, directory := range directories {
		mode, explicit := d.finalModes[directory]
		if !explicit {
			mode = 0o755
		}
		if err := os.Chmod(directory, mode); err != nil {
			return fmt.Errorf("apply directory mode to %q: %w", directory, err)
		}
	}
	return nil
}

func writeNewFile(
	destination string,
	mode fs.FileMode,
	overwrite bool,
	write func(io.Writer) error,
) (skipped bool, err error) {
	var (
		permissions fs.FileMode
		flags       int
		file        *os.File
	)

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return false, fmt.Errorf("create parent directories for %q: %w", destination, err)
	}
	permissions = mode.Perm() & 0o777
	if permissions == 0 {
		permissions = 0o644
	}
	flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		if info, statErr := os.Lstat(destination); statErr == nil && info.IsDir() {
			if removeErr := os.RemoveAll(destination); removeErr != nil {
				return false, fmt.Errorf("remove existing directory %q: %w", destination, removeErr)
			}
		}
	}
	file, err = os.OpenFile(destination, flags, permissions)
	if !overwrite && errors.Is(err, fs.ErrExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("create %q: %w", destination, err)
	}

	if writeErr := write(file); writeErr != nil {
		return false, removePartialFile(destination, file, writeErr)
	}
	if closeErr := file.Close(); closeErr != nil {
		if removeErr := os.Remove(destination); removeErr != nil {
			return false, errors.Join(closeErr, fmt.Errorf("remove partial file %q: %w", destination, removeErr))
		}
		return false, closeErr
	}
	return false, nil
}

func removePartialFile(destination string, file *os.File, writeErr error) error {
	var (
		closeErr  error
		removeErr error
	)

	closeErr = file.Close()
	removeErr = os.Remove(destination)
	if removeErr != nil {
		removeErr = fmt.Errorf("remove partial file %q: %w", destination, removeErr)
	}
	return errors.Join(writeErr, closeErr, removeErr)
}
