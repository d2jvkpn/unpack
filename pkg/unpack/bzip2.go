package unpack

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func openBzip2(path string) (io.Reader, func() error, error) {
	var (
		file         *os.File
		reader       io.Reader
		closeArchive func() error
		err          error
	)

	file, err = os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open Bzip2 file %q: %w", path, err)
	}
	reader, err = decompress(FormatBzip2, file)
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("open Bzip2 file %q: %w", path, err), file.Close())
	}
	closeArchive = func() error {
		var drainErr error

		_, drainErr = io.Copy(io.Discard, reader)
		return errors.Join(drainErr, file.Close())
	}
	return reader, closeArchive, nil
}

func scanBzip2(path string) (entries []Entry, err error) {
	var (
		reader       io.Reader
		closeArchive func() error
	)

	reader, closeArchive, err = openBzip2(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, closeArchive())
	}()
	if _, err = io.Copy(io.Discard, reader); err != nil {
		return nil, fmt.Errorf("read Bzip2 file %q: %w", path, err)
	}
	return []Entry{{
		Name:        archiveBaseName(path, FormatBzip2),
		Kind:        EntryFile,
		Mode:        0o644,
		SourceIndex: 0,
	}}, nil
}

func extractBzip2(path string, plans []Plan, overwrite bool) (skippedEntries []string, err error) {
	var (
		reader       io.Reader
		closeArchive func() error
		directories  *directoryFinalizer
	)

	if len(plans) == 0 {
		return nil, nil
	}
	if len(plans) != 1 || plans[0].Entry.SourceIndex != 0 || plans[0].Entry.Kind != EntryFile {
		return nil, fmt.Errorf("Bzip2 file %q has an invalid extraction plan", path)
	}
	reader, closeArchive, err = openBzip2(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, closeArchive())
	}()

	directories = newDirectoryFinalizer(overwrite)
	if err = directories.ensure(filepath.Dir(plans[0].Destination)); err != nil {
		return nil, fmt.Errorf("create parent directories for Bzip2 file %q: %w", plans[0].Entry.Name, err)
	}
	skipped, writeErr := writeNewFile(plans[0].Destination, fs.FileMode(0o644), overwrite, func(writer io.Writer) error {
		_, copyErr := io.Copy(writer, reader)
		return copyErr
	})
	if writeErr != nil {
		return nil, fmt.Errorf("extract Bzip2 file %q: %w", plans[0].Entry.Name, writeErr)
	}
	if skipped {
		skippedEntries = append(skippedEntries, plans[0].RelativeName)
	}
	return skippedEntries, nil
}
