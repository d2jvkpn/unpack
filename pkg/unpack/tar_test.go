package unpack

import (
	"archive/tar"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestScanTARClassifiesEntriesAcrossFormats(t *testing.T) {
	tests := []struct {
		name       string
		extension  string
		format     Format
		compressed bool
	}{
		{name: "tar", extension: ".tar", format: FormatTAR},
		{name: "tar gz", extension: ".tar.gz", format: FormatTarGzip, compressed: true},
		{name: "tgz", extension: ".tgz", format: FormatTarGzip, compressed: true},
	}
	want := []Entry{
		{Name: "notes.txt", Kind: EntryFile, Mode: 0o640, SourceIndex: 0},
		{Name: "docs/", Kind: EntryDirectory, Mode: 0o750, SourceIndex: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bundle"+tt.extension)
			writeTARFixture(t, path, tt.compressed, []tarFixture{
				{Name: "notes.txt", Body: "notes", Mode: 0o640, Typeflag: tar.TypeReg},
				{Name: "docs/", Mode: fs.ModeDir | 0o750, Typeflag: tar.TypeDir},
			})

			entries, err := scanTAR(path, tt.format, false)
			if err != nil || len(entries) != len(want) {
				t.Fatalf("scanTAR() = %#v, %v", entries, err)
			}
			for i := range want {
				if entries[i] != want[i] {
					t.Fatalf("entry %d = %#v, want %#v", i, entries[i], want[i])
				}
			}
		})
	}
}

func TestScanTARDecodesLegacyChineseName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.tar")
	raw := string([]byte{0xd6, 0xd0, 0xce, 0xc4, '.', 't', 'x', 't'})
	writeTARFixture(t, path, false, []tarFixture{{Name: raw, Body: "ok", Mode: 0o644, Typeflag: tar.TypeReg}})

	entries, err := scanTAR(path, FormatTAR, true)
	if err != nil || len(entries) != 1 || entries[0].Name != "中文.txt" {
		t.Fatalf("scanTAR() = %#v, %v", entries, err)
	}
}

func TestScanTARPreservesUTF8NameWithChineseEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "utf8.tar")
	writeTARFixture(t, path, false, []tarFixture{{Name: "中文.txt", Body: "ok", Mode: 0o644, Typeflag: tar.TypeReg}})

	entries, err := scanTAR(path, FormatTAR, true)
	if err != nil || len(entries) != 1 || entries[0].Name != "中文.txt" {
		t.Fatalf("scanTAR() = %#v, %v", entries, err)
	}
}

func TestScanTARRejectsHardLink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "link.tar")
	writeTARFixture(t, path, false, []tarFixture{{
		Name:     "linked",
		Mode:     0o777,
		Typeflag: tar.TypeLink,
		Linkname: "target",
	}})

	_, err := scanTAR(path, FormatTAR, false)
	if err == nil {
		t.Fatal("scanTAR() accepted a hard link entry")
	}
	if !errors.Is(err, ErrUnsafeEntry) {
		t.Fatalf("scanTAR() error = %v, want ErrUnsafeEntry", err)
	}
}

func TestScanTARClassifiesSymlinkEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "link.tar")
	writeTARFixture(t, path, false, []tarFixture{{
		Name:     "linked",
		Mode:     0o777,
		Typeflag: tar.TypeSymlink,
		Linkname: "target",
	}})

	entries, err := scanTAR(path, FormatTAR, false)
	if err != nil || len(entries) != 1 {
		t.Fatalf("scanTAR() = %#v, %v", entries, err)
	}
	if entries[0].Kind != EntrySymlink || entries[0].LinkTarget != "target" {
		t.Fatalf("entry = %#v, want symlink with LinkTarget %q", entries[0], "target")
	}
}

func TestScanTARRejectsInvalidGzipFooter(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{name: "checksum", mutate: corruptTARGzipChecksum},
		{name: "truncated", mutate: truncateTARGzipFooter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bundle.tar.gz")
			writeTARFixture(t, path, true, []tarFixture{{
				Name:     "note.txt",
				Body:     "content",
				Mode:     0o600,
				Typeflag: tar.TypeReg,
			}})
			tt.mutate(t, path)

			if _, err := scanTAR(path, FormatTarGzip, false); err == nil {
				t.Fatal("scanTAR() accepted a TAR.GZ archive with an invalid gzip footer")
			}
		})
	}
}

func TestExtractTARWritesPlannedEntriesAcrossFormats(t *testing.T) {
	tests := []struct {
		name       string
		extension  string
		format     Format
		compressed bool
	}{
		{name: "tar", extension: ".tar", format: FormatTAR},
		{name: "tar gz", extension: ".tar.gz", format: FormatTarGzip, compressed: true},
		{name: "tgz", extension: ".tgz", format: FormatTarGzip, compressed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "bundle"+tt.extension)
			writeTARFixture(t, path, tt.compressed, []tarFixture{
				{Name: "docs/", Mode: fs.ModeDir | 0o750, Typeflag: tar.TypeDir},
				{Name: "docs/note.txt", Body: "literal content", Mode: 0o600, Typeflag: tar.TypeReg},
			})

			entries, err := scanTAR(path, tt.format, false)
			if err != nil {
				t.Fatal(err)
			}
			plans, _, err := PlanEntries(entries, path, filepath.Join(root, "out"), root, tt.format)
			if err != nil {
				t.Fatal(err)
			}
			skipped, err := extractTAR(path, tt.format, plans, false)
			if err != nil || len(skipped) != 0 {
				t.Fatalf("extractTAR() = %#v, %v", skipped, err)
			}

			info, err := os.Stat(filepath.Join(root, "out"))
			if err != nil || !info.IsDir() || info.Mode().Perm() != 0o750 {
				t.Fatalf("extracted directory = %v, %v", info, err)
			}
			contents, err := os.ReadFile(filepath.Join(root, "out", "note.txt"))
			if err != nil || string(contents) != "literal content" {
				t.Fatalf("extracted contents = %q, %v", contents, err)
			}
		})
	}
}

func TestExtractTARSkipsExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bundle.tar")
	writeTARFixture(t, path, false, []tarFixture{{Name: "note.txt", Body: "replacement", Mode: 0o600, Typeflag: tar.TypeReg}})

	entries, err := scanTAR(path, FormatTAR, false)
	if err != nil {
		t.Fatal(err)
	}
	plans, _, err := PlanEntries(entries, path, filepath.Join(root, "out"), root, FormatTAR)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(plans[0].Destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plans[0].Destination, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	skipped, err := extractTAR(path, FormatTAR, plans, false)
	if err != nil || len(skipped) != 1 || skipped[0] != "note.txt" {
		t.Fatalf("extractTAR() = %#v, %v", skipped, err)
	}
	contents, err := os.ReadFile(plans[0].Destination)
	if err != nil || string(contents) != "original" {
		t.Fatalf("existing contents = %q, %v", contents, err)
	}
}

func TestExtractTARUsesPlannedSourceIndex(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bundle.tar")
	writeTARFixture(t, path, false, []tarFixture{
		{Name: "ignored.txt", Body: "ignore", Mode: 0o600, Typeflag: tar.TypeReg},
		{Name: "selected.txt", Body: "select", Mode: 0o600, Typeflag: tar.TypeReg},
	})

	plans := []Plan{{
		Entry:        Entry{Name: "selected.txt", Kind: EntryFile, Mode: 0o600, SourceIndex: 1},
		RelativeName: "selected.txt",
		Destination:  filepath.Join(root, "out", "selected.txt"),
	}}
	if _, err := extractTAR(path, FormatTAR, plans, false); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(plans[0].Destination)
	if err != nil || string(contents) != "select" {
		t.Fatalf("selected contents = %q, %v", contents, err)
	}
	if _, err := os.Stat(filepath.Join(root, "out", "ignored.txt")); !os.IsNotExist(err) {
		t.Fatalf("ignored entry was written: %v", err)
	}
}

func TestExtractTARErrorsForMissingPlannedSourceIndex(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bundle.tar")
	writeTARFixture(t, path, false, []tarFixture{{Name: "note.txt", Body: "content", Mode: 0o600, Typeflag: tar.TypeReg}})

	plans := []Plan{{
		Entry:        Entry{Name: "missing.txt", Kind: EntryFile, Mode: 0o600, SourceIndex: 1},
		RelativeName: "missing.txt",
		Destination:  filepath.Join(root, "out", "missing.txt"),
	}}
	if _, err := extractTAR(path, FormatTAR, plans, false); err == nil {
		t.Fatal("extractTAR() accepted a missing planned source index")
	}
}

func TestExtractTARDefersRestrictiveDirectoryModes(t *testing.T) {
	tests := []struct {
		name    string
		entries []tarFixture
		check   func(*testing.T, string)
	}{
		{
			name: "nested modes are finalized deepest first",
			entries: []tarFixture{
				{Name: "outer/", Mode: 0o400, Typeflag: tar.TypeDir},
				{Name: "outer/inner/", Mode: 0o400, Typeflag: tar.TypeDir},
				{Name: "outer/inner/note.txt", Body: "nested", Mode: 0o600, Typeflag: tar.TypeReg},
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
			entries: []tarFixture{
				{Name: "docs/note.txt", Body: "ordered", Mode: 0o600, Typeflag: tar.TypeReg},
				{Name: "docs/", Mode: 0o500, Typeflag: tar.TypeDir},
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
			archivePath := filepath.Join(root, "bundle.tar")
			writeTARFixture(t, archivePath, false, tt.entries)
			entries, err := scanTAR(archivePath, FormatTAR, false)
			if err != nil {
				t.Fatal(err)
			}
			plans, _, err := PlanEntries(entries, archivePath, "", root, FormatTAR)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := extractTAR(archivePath, FormatTAR, plans, false); err != nil {
				t.Fatal(err)
			}
			tt.check(t, root)
		})
	}
}

func TestExtractTARDoesNotChmodPreExistingDirectory(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	if err := os.Mkdir(docs, 0o750); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "bundle.tar")
	writeTARFixture(t, archivePath, false, []tarFixture{
		{Name: "docs/", Mode: 0o400, Typeflag: tar.TypeDir},
		{Name: "docs/note.txt", Body: "content", Mode: 0o600, Typeflag: tar.TypeReg},
	})
	entries, err := scanTAR(archivePath, FormatTAR, false)
	if err != nil {
		t.Fatal(err)
	}
	plans, _, err := PlanEntries(entries, archivePath, "", root, FormatTAR)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := extractTAR(archivePath, FormatTAR, plans, false); err != nil {
		t.Fatal(err)
	}
	assertDirectoryMode(t, docs, 0o750)
}
