package unpack

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"
)

func openTAR(path string, format archiveFormat) (*tar.Reader, func() error, error) {
	var (
		file         *os.File
		reader       io.Reader
		gzipReader   *gzip.Reader
		closeArchive func() error
		err          error
	)

	file, err = os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open TAR archive %q: %w", path, err)
	}

	reader = file
	switch format {
	case formatTAR:
	case formatTarGzip:
		gzipReader, err = gzip.NewReader(file)
		if err != nil {
			return nil, nil, errors.Join(fmt.Errorf("open gzip TAR archive %q: %w", path, err), file.Close())
		}
		reader = gzipReader
	default:
		return nil, nil, errors.Join(fmt.Errorf("unsupported TAR format %d", format), file.Close())
	}

	closeArchive = func() error {
		var (
			drainErr     error
			gzipCloseErr error
			fileCloseErr error
		)

		if gzipReader == nil {
			return file.Close()
		}
		_, drainErr = io.Copy(io.Discard, gzipReader)
		gzipCloseErr = gzipReader.Close()
		fileCloseErr = file.Close()
		return errors.Join(drainErr, gzipCloseErr, fileCloseErr)
	}
	return tar.NewReader(reader), closeArchive, nil
}

func scanTAR(path string, format archiveFormat, chinese bool) (entries []archiveEntry, err error) {
	var (
		reader       *tar.Reader
		closeArchive func() error
	)

	reader, closeArchive, err = openTAR(path, format)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, closeArchive())
	}()

	for index := 0; ; index++ {
		header, nextErr := reader.Next()
		if errors.Is(nextErr, io.EOF) {
			return entries, nil
		}
		if nextErr != nil {
			return nil, fmt.Errorf("read TAR archive %q: %w", path, nextErr)
		}

		name, decodeErr := decodeArchiveName(header.Name, chinese, !utf8.ValidString(header.Name))
		if decodeErr != nil {
			return nil, fmt.Errorf("decode TAR entry %q: %w", header.Name, decodeErr)
		}

		kind := entryKind(0)
		var linkTarget string
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			kind = entryFile
		case tar.TypeDir:
			kind = entryDirectory
		case tar.TypeSymlink:
			kind = entrySymlink
			linkTarget = header.Linkname
		default:
			return nil, fmt.Errorf("unsafe TAR entry %q: unsupported type %q", name, header.Typeflag)
		}
		entries = append(entries, archiveEntry{
			Name:        name,
			Kind:        kind,
			Mode:        fs.FileMode(header.Mode).Perm(),
			LinkTarget:  linkTarget,
			SourceIndex: index,
		})
	}
}

func extractTAR(
	path string,
	format archiveFormat,
	plans []plannedEntry,
	overwrite bool,
) (skippedEntries []string, err error) {
	var (
		reader       *tar.Reader
		closeArchive func() error
		nextPlan     int
		directories  *directoryFinalizer
	)

	reader, closeArchive, err = openTAR(path, format)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, closeArchive())
	}()

	for _, plan := range plans {
		if plan.Entry.SourceIndex < 0 {
			return nil, fmt.Errorf("TAR entry %q has invalid source index %d", plan.Entry.Name, plan.Entry.SourceIndex)
		}
		switch plan.Entry.Kind {
		case entryFile, entryDirectory, entrySymlink:
		default:
			return nil, fmt.Errorf("TAR entry %q has unsupported entry kind %d", plan.Entry.Name, plan.Entry.Kind)
		}
	}

	directories = newDirectoryFinalizer(overwrite)
	for index := 0; ; index++ {
		_, nextErr := reader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return skippedEntries, fmt.Errorf("read TAR archive %q: %w", path, nextErr)
		}
		if nextPlan == len(plans) {
			continue
		}

		plan := plans[nextPlan]
		if plan.Entry.SourceIndex < index {
			return skippedEntries, fmt.Errorf(
				"TAR entry %q has missing source index %d",
				plan.Entry.Name,
				plan.Entry.SourceIndex,
			)
		}
		if plan.Entry.SourceIndex != index {
			continue
		}

		switch plan.Entry.Kind {
		case entryDirectory:
			if mkdirErr := directories.ensure(plan.Destination); mkdirErr != nil {
				return skippedEntries, fmt.Errorf("create directory for TAR entry %q: %w", plan.Entry.Name, mkdirErr)
			}
			directories.recordMode(plan.Destination, plan.Entry.Mode)
		case entryFile:
			if mkdirErr := directories.ensure(filepath.Dir(plan.Destination)); mkdirErr != nil {
				return skippedEntries, fmt.Errorf("create parent directories for TAR entry %q: %w", plan.Entry.Name, mkdirErr)
			}
			skipped, writeErr := writeNewFile(plan.Destination, plan.Entry.Mode, overwrite, func(writer io.Writer) error {
				var copyErr error

				_, copyErr = io.Copy(writer, reader)
				return copyErr
			})
			if writeErr != nil {
				return skippedEntries, fmt.Errorf("extract TAR entry %q: %w", plan.Entry.Name, writeErr)
			}
			if skipped {
				skippedEntries = append(skippedEntries, plan.RelativeName)
			}
		case entrySymlink:
			if mkdirErr := directories.ensure(filepath.Dir(plan.Destination)); mkdirErr != nil {
				return skippedEntries, fmt.Errorf("create parent directories for TAR entry %q: %w", plan.Entry.Name, mkdirErr)
			}
			skipped, symlinkErr := writeSymlink(plan.Destination, plan.Entry.LinkTarget, overwrite)
			if symlinkErr != nil {
				return skippedEntries, fmt.Errorf("extract TAR entry %q: %w", plan.Entry.Name, symlinkErr)
			}
			if skipped {
				skippedEntries = append(skippedEntries, plan.RelativeName)
			}
		}
		nextPlan++
	}

	if nextPlan != len(plans) {
		plan := plans[nextPlan]
		return skippedEntries, fmt.Errorf("TAR entry %q has missing source index %d", plan.Entry.Name, plan.Entry.SourceIndex)
	}
	if finalizeErr := directories.finalize(); finalizeErr != nil {
		return skippedEntries, fmt.Errorf("finalize TAR directory modes: %w", finalizeErr)
	}
	return skippedEntries, nil
}
