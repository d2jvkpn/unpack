package unpack

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
)

const maxSymlinkTargetBytes = 4096

func scanZIP(path string, chinese bool) (entries []Entry, err error) {
	var reader *zip.ReadCloser

	reader, err = zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open ZIP archive %q: %w", path, err)
	}
	defer joinCloseError(&err, reader)

	entries = make([]Entry, 0, len(reader.File))
	for index, file := range reader.File {
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
		entries = append(entries, Entry{
			Name:        name,
			Kind:        kind,
			Mode:        file.Mode().Perm(),
			LinkTarget:  linkTarget,
			SourceIndex: index,
		})
	}
	return entries, nil
}

func extractZIP(path string, plans []Plan, overwrite bool) (skippedEntries []string, err error) {
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
		case EntryFile, EntryDirectory, EntrySymlink:
		default:
			return nil, fmt.Errorf("ZIP entry %q has unsupported entry kind %d", plan.Entry.Name, plan.Entry.Kind)
		}
	}

	skippedEntries = make([]string, 0)
	directories = newDirectoryFinalizer(overwrite)
	for _, plan := range plans {
		file := reader.File[plan.Entry.SourceIndex]
		switch plan.Entry.Kind {
		case EntryDirectory:
			if err := directories.ensure(plan.Destination); err != nil {
				return skippedEntries, fmt.Errorf("create directory for ZIP entry %q: %w", plan.Entry.Name, err)
			}
			directories.recordMode(plan.Destination, plan.Entry.Mode)
		case EntryFile:
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
		case EntrySymlink:
			if err := directories.ensure(filepath.Dir(plan.Destination)); err != nil {
				return skippedEntries, fmt.Errorf("create parent directories for ZIP entry %q: %w", plan.Entry.Name, err)
			}
			skipped, err := writeSymlink(plan.Destination, plan.Entry.LinkTarget, overwrite)
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

func zipEntryKind(file *zip.File) (EntryKind, error) {
	switch file.Mode().Type() {
	case fs.ModeDir:
		return EntryDirectory, nil
	case fs.ModeSymlink:
		if strings.HasSuffix(file.Name, "/") {
			return 0, fmt.Errorf("symlink entry has directory-style name %q", file.Name)
		}
		return EntrySymlink, nil
	case 0:
		return EntryFile, nil
	default:
		return 0, fmt.Errorf("unsupported file mode %v", file.Mode())
	}
}

func readZIPSymlinkTarget(file *zip.File) (target string, err error) {
	var (
		reader io.ReadCloser
		data   []byte
	)

	reader, err = file.Open()
	if err != nil {
		return "", fmt.Errorf("open symlink entry: %w", err)
	}
	defer joinCloseError(&err, reader)

	data, err = io.ReadAll(io.LimitReader(reader, maxSymlinkTargetBytes+1))
	if err != nil {
		return "", fmt.Errorf("read symlink target: %w", err)
	}
	if len(data) > maxSymlinkTargetBytes {
		return "", fmt.Errorf("symlink target exceeds %d bytes", maxSymlinkTargetBytes)
	}
	return string(data), nil
}

func directoryMode(mode fs.FileMode) fs.FileMode {
	var permissions fs.FileMode

	permissions = mode.Perm() & 0o777
	if permissions == 0 {
		return 0o755
	}
	return permissions
}
