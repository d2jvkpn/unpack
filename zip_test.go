package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type closeError struct {
	err error
}

func (c closeError) Close() error {
	return c.err
}

func TestJoinCloseErrorPreservesOperationAndCloseFailures(t *testing.T) {
	operationErr := errors.New("operation failed")
	containerCloseErr := errors.New("container close failed")
	got := operationErr

	joinCloseError(&got, closeError{err: containerCloseErr})

	if !errors.Is(got, operationErr) || !errors.Is(got, containerCloseErr) {
		t.Fatalf("joined error = %v, want operation and close failures", got)
	}
}

func TestScanZIPDecodesLegacyChineseName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.zip")
	raw := string([]byte{0xd6, 0xd0, 0xce, 0xc4, '.', 't', 'x', 't'})
	writeZIPFixture(t, path, []zipFixture{{Name: raw, NonUTF8: true, Body: "ok"}})

	entries, err := scanZIP(path, true)
	if err != nil || len(entries) != 1 || entries[0].Name != "中文.txt" {
		t.Fatalf("scanZIP() = %#v, %v", entries, err)
	}
}

func TestScanZIPClassifiesEntriesAndRecordsSourceIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "entries.zip")
	writeZIPFixture(t, path, []zipFixture{
		{Name: "notes.txt", Body: "notes", Mode: 0o640},
		{Name: "docs/", Mode: fs.ModeDir | 0o750},
	})

	entries, err := scanZIP(path, false)
	want := []archiveEntry{
		{Name: "notes.txt", Kind: entryFile, Mode: 0o640, SourceIndex: 0},
		{Name: "docs/", Kind: entryDirectory, Mode: 0o750, SourceIndex: 1},
	}
	if err != nil || len(entries) != len(want) {
		t.Fatalf("scanZIP() = %#v, %v", entries, err)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Fatalf("entry %d = %#v, want %#v", i, entries[i], want[i])
		}
	}
}

func TestScanZIPPreservesUTF8NameWithChineseEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "utf8.zip")
	writeZIPFixture(t, path, []zipFixture{{Name: "中文.txt", Body: "ok"}})

	entries, err := scanZIP(path, true)
	if err != nil || len(entries) != 1 || entries[0].Name != "中文.txt" {
		t.Fatalf("scanZIP() = %#v, %v", entries, err)
	}
}

func TestScanZIPUsesUTF8MarkerInsteadOfByteValidity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marked-invalid-utf8.zip")
	raw := string([]byte{0x81})
	writeZIPFixture(t, path, []zipFixture{{Name: raw, Flags: 0x800, Body: "ok"}})

	entries, err := scanZIP(path, true)
	if err != nil || len(entries) != 1 || entries[0].Name != raw {
		t.Fatalf("scanZIP() = %#v, %v; want UTF-8-marked name bytes preserved", entries, err)
	}
}

func TestScanZIPRejectsEncryptedEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "encrypted.zip")
	writeZIPFixture(t, path, []zipFixture{{Name: "secret.txt", Body: "secret"}})
	patchZIPEncrypted(t, path)

	if _, err := scanZIP(path, false); err == nil {
		t.Fatal("scanZIP() accepted an encrypted entry")
	}
}

func TestScanZIPRejectsSymlink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "symlink.zip")
	writeZIPFixture(t, path, []zipFixture{{
		Name: "linked",
		Body: "target",
		Mode: fs.ModeSymlink | 0o777,
	}})

	if _, err := scanZIP(path, false); err == nil {
		t.Fatal("scanZIP() accepted a symbolic link")
	}
}

func TestScanZIPRejectsTrailingSlashSymlink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trailing-slash-symlink.zip")
	writeZIPFixture(t, path, []zipFixture{{
		Name: "linked/",
		Mode: fs.ModeSymlink | 0o777,
	}})

	if _, err := scanZIP(path, false); err == nil {
		t.Fatal("scanZIP() accepted a trailing-slash symbolic link")
	}
}

func TestExtractZIPWritesPlannedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bundle.zip")
	writeZIPFixture(t, path, []zipFixture{{Name: "note.txt", Body: "literal content", Mode: 0o600}})

	entries, err := scanZIP(path, false)
	if err != nil {
		t.Fatal(err)
	}
	plans, _, err := planEntries(entries, path, filepath.Join(root, "out"), root, formatZIP)
	if err != nil {
		t.Fatal(err)
	}
	skipped, err := extractZIP(path, plans, false)
	if err != nil || len(skipped) != 0 {
		t.Fatalf("extractZIP() = %#v, %v", skipped, err)
	}

	destination := filepath.Join(root, "out", "note.txt")
	contents, err := os.ReadFile(destination)
	if err != nil || string(contents) != "literal content" {
		t.Fatalf("extracted contents = %q, %v", contents, err)
	}
	info, err := os.Stat(destination)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("extracted mode = %v, %v", info.Mode(), err)
	}
}

func TestExtractZIPSkipsExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bundle.zip")
	writeZIPFixture(t, path, []zipFixture{{Name: "note.txt", Body: "replacement"}})

	entries, err := scanZIP(path, false)
	if err != nil {
		t.Fatal(err)
	}
	plans, _, err := planEntries(entries, path, filepath.Join(root, "out"), root, formatZIP)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(plans[0].Destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plans[0].Destination, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	skipped, err := extractZIP(path, plans, false)
	if err != nil || len(skipped) != 1 || skipped[0] != "note.txt" {
		t.Fatalf("extractZIP() = %#v, %v", skipped, err)
	}
	contents, err := os.ReadFile(plans[0].Destination)
	if err != nil || string(contents) != "original" {
		t.Fatalf("existing contents = %q, %v", contents, err)
	}
}

func TestExtractZIPDefersRestrictiveDirectoryModes(t *testing.T) {
	tests := []struct {
		name    string
		entries []zipFixture
		check   func(*testing.T, string)
	}{
		{
			name: "nested modes are finalized deepest first",
			entries: []zipFixture{
				{Name: "outer/", Mode: fs.ModeDir | 0o400},
				{Name: "outer/inner/", Mode: fs.ModeDir | 0o400},
				{Name: "outer/inner/note.txt", Body: "nested"},
			},
			check: func(t *testing.T, root string) {
				outer := filepath.Join(root, "outer")
				inner := filepath.Join(outer, "inner")
				t.Cleanup(func() {
					_ = os.Chmod(outer, 0o700)
					_ = os.Chmod(inner, 0o700)
				})
				assertDirectoryMode(t, outer, 0o400)
				if err := os.Chmod(outer, 0o700); err != nil {
					t.Fatal(err)
				}
				assertDirectoryMode(t, inner, 0o400)
				if err := os.Chmod(inner, 0o500); err != nil {
					t.Fatal(err)
				}
				assertFileContents(t, filepath.Join(inner, "note.txt"), "nested")
			},
		},
		{
			name: "file before directory marker",
			entries: []zipFixture{
				{Name: "docs/note.txt", Body: "ordered"},
				{Name: "docs/", Mode: fs.ModeDir | 0o500},
			},
			check: func(t *testing.T, root string) {
				docs := filepath.Join(root, "docs")
				t.Cleanup(func() { _ = os.Chmod(docs, 0o700) })
				assertDirectoryMode(t, docs, 0o500)
				assertFileContents(t, filepath.Join(docs, "note.txt"), "ordered")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			archivePath := filepath.Join(root, "bundle.zip")
			writeZIPFixture(t, archivePath, tt.entries)
			entries, err := scanZIP(archivePath, false)
			if err != nil {
				t.Fatal(err)
			}
			plans, _, err := planEntries(entries, archivePath, "", root, formatZIP)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := extractZIP(archivePath, plans, false); err != nil {
				t.Fatal(err)
			}
			tt.check(t, root)
		})
	}
}

func TestExtractZIPDoesNotChmodPreExistingDirectory(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	if err := os.Mkdir(docs, 0o750); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "bundle.zip")
	writeZIPFixture(t, archivePath, []zipFixture{
		{Name: "docs/", Mode: fs.ModeDir | 0o400},
		{Name: "docs/note.txt", Body: "content"},
	})
	entries, err := scanZIP(archivePath, false)
	if err != nil {
		t.Fatal(err)
	}
	plans, _, err := planEntries(entries, archivePath, "", root, formatZIP)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := extractZIP(archivePath, plans, false); err != nil {
		t.Fatal(err)
	}
	assertDirectoryMode(t, docs, 0o750)
}

func assertDirectoryMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("directory %q mode = %v, want %v", path, got, want)
	}
}
