package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
)

func scanZIP(path string, chinese bool) (entries []archiveEntry, err error) {
	var reader *zip.ReadCloser

	reader, err = zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open ZIP archive %q: %w", path, err)
	}
	defer joinCloseError(&err, reader)

	entries = make([]archiveEntry, 0, len(reader.File))
	for index, file := range reader.File {
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
		entries = append(entries, archiveEntry{
			Name:        name,
			Kind:        kind,
			Mode:        file.Mode().Perm(),
			SourceIndex: index,
		})
	}
	return entries, nil
}

func extractZIP(path string, plans []plannedEntry, overwrite bool) (skippedEntries []string, err error) {
	var (
		reader      *zip.ReadCloser
		directories *directoryFinalizer
	)

	reader, err = zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open ZIP archive %q: %w", path, err)
	}
	defer joinCloseError(&err, reader)

	for _, plan := range plans {
		if plan.Entry.SourceIndex < 0 || plan.Entry.SourceIndex >= len(reader.File) {
			return nil, fmt.Errorf("ZIP entry %q has invalid source index %d", plan.Entry.Name, plan.Entry.SourceIndex)
		}
		switch plan.Entry.Kind {
		case entryFile, entryDirectory:
		default:
			return nil, fmt.Errorf("ZIP entry %q has unsupported entry kind %d", plan.Entry.Name, plan.Entry.Kind)
		}
	}

	skippedEntries = make([]string, 0)
	directories = newDirectoryFinalizer(overwrite)
	for _, plan := range plans {
		file := reader.File[plan.Entry.SourceIndex]
		switch plan.Entry.Kind {
		case entryDirectory:
			if err := directories.ensure(plan.Destination); err != nil {
				return skippedEntries, fmt.Errorf("create directory for ZIP entry %q: %w", plan.Entry.Name, err)
			}
			directories.recordMode(plan.Destination, plan.Entry.Mode)
		case entryFile:
			if err := directories.ensure(filepath.Dir(plan.Destination)); err != nil {
				return skippedEntries, fmt.Errorf("create parent directories for ZIP entry %q: %w", plan.Entry.Name, err)
			}
			skipped, err := writeNewFile(plan.Destination, plan.Entry.Mode, overwrite, func(writer io.Writer) error {
				var (
					entryReader io.ReadCloser
					copyErr     error
					closeErr    error
					err         error
				)

				entryReader, err = file.Open()
				if err != nil {
					return fmt.Errorf("open ZIP entry %q: %w", plan.Entry.Name, err)
				}
				_, copyErr = io.Copy(writer, entryReader)
				closeErr = entryReader.Close()
				return errors.Join(copyErr, closeErr)
			})
			if err != nil {
				return skippedEntries, fmt.Errorf("extract ZIP entry %q: %w", plan.Entry.Name, err)
			}
			if skipped {
				skippedEntries = append(skippedEntries, plan.RelativeName)
			}
		}
	}
	if err := directories.finalize(); err != nil {
		return skippedEntries, fmt.Errorf("finalize ZIP directory modes: %w", err)
	}
	return skippedEntries, nil
}

func joinCloseError(err *error, closer io.Closer) {
	*err = errors.Join(*err, closer.Close())
}

func zipEntryKind(file *zip.File) (entryKind, error) {
	switch file.Mode().Type() {
	case fs.ModeDir:
		return entryDirectory, nil
	case 0:
		return entryFile, nil
	default:
		return 0, fmt.Errorf("unsupported file mode %v", file.Mode())
	}
}

func directoryMode(mode fs.FileMode) fs.FileMode {
	var permissions fs.FileMode

	permissions = mode.Perm() & 0o777
	if permissions == 0 {
		return 0o755
	}
	return permissions
}
