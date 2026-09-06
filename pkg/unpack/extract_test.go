package unpack

import (
	"archive/tar"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractUsesSmartDefaultDestinations(t *testing.T) {
	t.Run("one top-level item extracts directly", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "archives", "single.zip")
		if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
			t.Fatal(err)
		}
		writeZIPFixture(t, archivePath, []zipFixture{{Name: "note.txt", Body: "single"}})
		workingDir := filepath.Join(root, "working")
		if err := os.Mkdir(workingDir, 0o755); err != nil {
			t.Fatal(err)
		}

		_, err := Extract(archivePath, Options{WorkingDir: workingDir})

		if err != nil {
			t.Fatalf("Extract() error = %v", err)
		}
		assertFileContents(t, filepath.Join(workingDir, "note.txt"), "single")
		assertPathDoesNotExist(t, filepath.Join(workingDir, "single"))
	})

	t.Run("ZIP with multiple top-level items uses archive basename", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "archives", "bundle.zip")
		if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
			t.Fatal(err)
		}
		writeZIPFixture(t, archivePath, []zipFixture{
			{Name: "first.txt", Body: "first"},
			{Name: "docs/second.txt", Body: "second"},
		})
		workingDir := filepath.Join(root, "working")
		if err := os.Mkdir(workingDir, 0o755); err != nil {
			t.Fatal(err)
		}

		_, err := Extract(archivePath, Options{WorkingDir: workingDir})

		if err != nil {
			t.Fatalf("Extract() error = %v", err)
		}
		assertFileContents(t, filepath.Join(workingDir, "bundle", "first.txt"), "first")
		assertFileContents(t, filepath.Join(workingDir, "bundle", "docs", "second.txt"), "second")
	})

	t.Run("TAR.GZ removes the complete recognized suffix", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "archives", "bundle.tar.gz")
		if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
			t.Fatal(err)
		}
		writeTARFixture(t, archivePath, true, []tarFixture{
			{Name: "first.txt", Body: "first", Mode: 0o600, Typeflag: tar.TypeReg},
			{Name: "docs/second.txt", Body: "second", Mode: 0o600, Typeflag: tar.TypeReg},
		})
		workingDir := filepath.Join(root, "working")
		if err := os.Mkdir(workingDir, 0o755); err != nil {
			t.Fatal(err)
		}

		_, err := Extract(archivePath, Options{WorkingDir: workingDir})

		if err != nil {
			t.Fatalf("Extract() error = %v", err)
		}
		assertFileContents(t, filepath.Join(workingDir, "bundle", "first.txt"), "first")
		assertFileContents(t, filepath.Join(workingDir, "bundle", "docs", "second.txt"), "second")
		assertPathDoesNotExist(t, filepath.Join(workingDir, "bundle.tar"))
	})
}

func TestExtractRejectsUnsafeSyntheticDefaultBases(t *testing.T) {
	tests := []struct {
		archiveName string
		payloadPath func(root string, workingDir string) string
	}{
		{
			archiveName: ".zip",
			payloadPath: func(_ string, workingDir string) string {
				return filepath.Join(workingDir, "first.txt")
			},
		},
		{
			archiveName: "..zip",
			payloadPath: func(_ string, workingDir string) string {
				return filepath.Join(workingDir, "first.txt")
			},
		},
		{
			archiveName: "...zip",
			payloadPath: func(root string, _ string) string {
				return filepath.Join(root, "first.txt")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.archiveName, func(t *testing.T) {
			root := t.TempDir()
			archiveDir := filepath.Join(root, "archives")
			if err := os.Mkdir(archiveDir, 0o755); err != nil {
				t.Fatal(err)
			}
			archivePath := filepath.Join(archiveDir, tt.archiveName)
			writeZIPFixture(t, archivePath, []zipFixture{
				{Name: "first.txt", Body: "first"},
				{Name: "docs/second.txt", Body: "second"},
			})
			workingDir := filepath.Join(root, "working")
			if err := os.Mkdir(workingDir, 0o755); err != nil {
				t.Fatal(err)
			}

			_, err := Extract(archivePath, Options{WorkingDir: workingDir})

			if err == nil {
				t.Fatal("Extract() succeeded")
			}
			assertPathDoesNotExist(t, tt.payloadPath(root, workingDir))
		})
	}
}

func TestExtractRejectsSyntheticDefaultBaseSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "archives", "bundle.zip")
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeZIPFixture(t, archivePath, []zipFixture{
		{Name: "first.txt", Body: "first"},
		{Name: "docs/second.txt", Body: "second"},
	})
	workingDir := filepath.Join(root, "working")
	if err := os.Mkdir(workingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideDir := filepath.Join(root, "outside")
	if err := os.Mkdir(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(workingDir, "bundle")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	_, err := Extract(archivePath, Options{WorkingDir: workingDir})

	if err == nil {
		t.Fatal("Extract() succeeded")
	}
	assertPathDoesNotExist(t, filepath.Join(outsideDir, "first.txt"))
	assertPathDoesNotExist(t, filepath.Join(outsideDir, "docs", "second.txt"))
}

func TestExtractAcceptsRootDirectoryMarkers(t *testing.T) {
	tests := []struct {
		name        string
		extension   string
		write       func(*testing.T, string)
		payloadName string
	}{
		{
			name:      "ZIP",
			extension: ".zip",
			write: func(t *testing.T, archivePath string) {
				writeZIPFixture(t, archivePath, []zipFixture{
					{Name: ".", Mode: fs.ModeDir | 0o500},
					{Name: "zip.txt", Body: "zip"},
				})
			},
			payloadName: "zip.txt",
		},
		{
			name:      "TAR",
			extension: ".tar",
			write: func(t *testing.T, archivePath string) {
				writeTARFixture(t, archivePath, false, []tarFixture{
					{Name: ".", Mode: 0o500, Typeflag: tar.TypeDir},
					{Name: "tar.txt", Body: "tar", Mode: 0o600, Typeflag: tar.TypeReg},
				})
			},
			payloadName: "tar.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			archivePath := filepath.Join(root, "bundle"+tt.extension)
			tt.write(t, archivePath)
			outputDir := filepath.Join(root, "output")

			_, err := Extract(archivePath, Options{Directory: outputDir, WorkingDir: root})

			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}
			assertFileContents(t, filepath.Join(outputDir, tt.payloadName), strings.TrimSuffix(tt.payloadName, ".txt"))
		})
	}
}

func TestExtractExplicitOutputStripsSoleTopLevelDirectory(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "bundle.zip")
	writeZIPFixture(t, archivePath, []zipFixture{
		{Name: "wrapper/", Mode: fs.ModeDir | 0o755},
		{Name: "wrapper/note.txt", Body: "unwrapped"},
	})
	outputDir := filepath.Join(root, "output")

	_, err := Extract(archivePath, Options{Directory: outputDir, WorkingDir: root})

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	assertFileContents(t, filepath.Join(outputDir, "note.txt"), "unwrapped")
	assertPathDoesNotExist(t, filepath.Join(outputDir, "wrapper"))
}

func TestExtractSupportsBothFormats(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "first.zip")
	writeZIPFixture(t, zipPath, []zipFixture{{Name: "zip.txt", Body: "zip"}})
	tgzPath := filepath.Join(root, "second.tgz")
	writeTARFixture(t, tgzPath, true, []tarFixture{{
		Name: "tgz.txt", Body: "tgz", Mode: 0o600, Typeflag: tar.TypeReg,
	}})
	outputDir := filepath.Join(root, "output")

	for _, tt := range []struct {
		archivePath string
		fileName    string
		body        string
	}{
		{zipPath, "zip.txt", "zip"},
		{tgzPath, "tgz.txt", "tgz"},
	} {
		_, err := Extract(tt.archivePath, Options{Directory: outputDir, WorkingDir: root})
		if err != nil {
			t.Fatalf("Extract() error = %v", err)
		}
		assertFileContents(t, filepath.Join(outputDir, tt.fileName), tt.body)
	}
}

func TestExtractOnlySelectedFiles(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "bundle.zip")
	writeZIPFixture(t, archivePath, []zipFixture{
		{Name: "first.txt", Body: "first"},
		{Name: "second.txt", Body: "second"},
		{Name: "docs/third.txt", Body: "third"},
	})
	outputDir := filepath.Join(root, "output")

	_, err := Extract(archivePath, Options{
		Directory: outputDir, WorkingDir: root, Selectors: []string{"first.txt", "docs"},
	})

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	assertFileContents(t, filepath.Join(outputDir, "first.txt"), "first")
	assertFileContents(t, filepath.Join(outputDir, "docs", "third.txt"), "third")
	assertPathDoesNotExist(t, filepath.Join(outputDir, "second.txt"))
}

func TestExtractSelectorMatchesRelativeToSoleTopLevelDirectory(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "bundle.zip")
	writeZIPFixture(t, archivePath, []zipFixture{
		{Name: "wrapper/src/main.go", Body: "main"},
		{Name: "wrapper/README.md", Body: "readme"},
	})
	workingDir := filepath.Join(root, "working")
	if err := os.Mkdir(workingDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Extract(archivePath, Options{WorkingDir: workingDir, Selectors: []string{"src/main.go"}})

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	assertFileContents(t, filepath.Join(workingDir, "wrapper", "src", "main.go"), "main")
	assertPathDoesNotExist(t, filepath.Join(workingDir, "wrapper", "README.md"))
}

func TestExtractFailsWithoutWritingWhenSelectorMatchesNothing(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "bundle.zip")
	writeZIPFixture(t, archivePath, []zipFixture{{Name: "first.txt", Body: "first"}})
	outputDir := filepath.Join(root, "output")

	_, err := Extract(archivePath, Options{
		Directory: outputDir, WorkingDir: root, Selectors: []string{"missing.txt"},
	})

	if err == nil {
		t.Fatal("Extract() succeeded")
	}
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("Extract() error = %v, want ErrNoMatch", err)
	}
	assertPathDoesNotExist(t, filepath.Join(outputDir, "first.txt"))
}

func TestExtractRejectsUnsafeArchivesBeforeWritingPayloads(t *testing.T) {
	t.Run("path traversal", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "traversal.zip")
		writeZIPFixture(t, archivePath, []zipFixture{
			{Name: "safe.txt", Body: "must not be written"},
			{Name: "../escape.txt", Body: "escape"},
		})
		outputDir := filepath.Join(root, "output")

		_, err := Extract(archivePath, Options{Directory: outputDir, WorkingDir: root})

		if err == nil {
			t.Fatal("Extract() succeeded")
		}
		if !errors.Is(err, ErrUnsafeEntry) {
			t.Fatalf("Extract() error = %v, want ErrUnsafeEntry", err)
		}
		assertPathDoesNotExist(t, filepath.Join(outputDir, "safe.txt"))
		assertPathDoesNotExist(t, filepath.Join(root, "escape.txt"))
	})

	t.Run("existing directory symlink escape", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "symlink-escape.zip")
		writeZIPFixture(t, archivePath, []zipFixture{
			{Name: "safe.txt", Body: "must not be written"},
			{Name: "linked/escape.txt", Body: "escape"},
		})
		outputDir := filepath.Join(root, "output")
		if err := os.Mkdir(outputDir, 0o755); err != nil {
			t.Fatal(err)
		}
		outsideDir := filepath.Join(root, "outside")
		if err := os.Mkdir(outsideDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outsideDir, filepath.Join(outputDir, "linked")); err != nil {
			t.Skipf("cannot create symlink: %v", err)
		}

		_, err := Extract(archivePath, Options{Directory: outputDir, WorkingDir: root})

		if err == nil {
			t.Fatal("Extract() succeeded")
		}
		if !errors.Is(err, ErrUnsafeEntry) {
			t.Fatalf("Extract() error = %v, want ErrUnsafeEntry", err)
		}
		assertPathDoesNotExist(t, filepath.Join(outputDir, "safe.txt"))
		assertPathDoesNotExist(t, filepath.Join(outsideDir, "escape.txt"))
	})

	t.Run("TAR hard link", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "links.tar")
		writeTARFixture(t, archivePath, false, []tarFixture{
			{Name: "safe.txt", Body: "must not be written", Mode: 0o600, Typeflag: tar.TypeReg},
			{Name: "linked", Mode: 0o777, Typeflag: tar.TypeLink, Linkname: "safe.txt"},
		})
		outputDir := filepath.Join(root, "output")

		_, err := Extract(archivePath, Options{Directory: outputDir, WorkingDir: root})

		if err == nil {
			t.Fatal("Extract() succeeded")
		}
		assertPathDoesNotExist(t, filepath.Join(outputDir, "safe.txt"))
		assertPathDoesNotExist(t, filepath.Join(outputDir, "linked"))
	})

	t.Run("TAR symlink target escapes extraction root", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "escape-link.tar")
		writeTARFixture(t, archivePath, false, []tarFixture{
			{Name: "safe.txt", Body: "must not be written", Mode: 0o600, Typeflag: tar.TypeReg},
			{Name: "linked", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "../../etc/passwd"},
		})
		outputDir := filepath.Join(root, "output")

		_, err := Extract(archivePath, Options{Directory: outputDir, WorkingDir: root})

		if err == nil {
			t.Fatal("Extract() succeeded")
		}
		if !errors.Is(err, ErrUnsafeEntry) {
			t.Fatalf("Extract() error = %v, want ErrUnsafeEntry", err)
		}
		assertPathDoesNotExist(t, filepath.Join(outputDir, "safe.txt"))
		assertPathDoesNotExist(t, filepath.Join(outputDir, "linked"))
	})

	t.Run("TAR symlink is an ancestor of another entry", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "ancestor-link.tar")
		writeTARFixture(t, archivePath, false, []tarFixture{
			{Name: "linked", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "outside"},
			{Name: "linked/escape.txt", Body: "must not be written", Mode: 0o600, Typeflag: tar.TypeReg},
		})
		outputDir := filepath.Join(root, "output")

		_, err := Extract(archivePath, Options{Directory: outputDir, WorkingDir: root})

		if err == nil {
			t.Fatal("Extract() succeeded")
		}
		if !errors.Is(err, ErrUnsafeEntry) {
			t.Fatalf("Extract() error = %v, want ErrUnsafeEntry", err)
		}
		assertPathDoesNotExist(t, filepath.Join(outputDir, "linked"))
	})

	t.Run("encrypted ZIP", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "encrypted.zip")
		writeZIPFixture(t, archivePath, []zipFixture{{Name: "secret.txt", Body: "secret"}})
		patchZIPEncrypted(t, archivePath)
		outputDir := filepath.Join(root, "output")

		_, err := Extract(archivePath, Options{Directory: outputDir, WorkingDir: root})

		if err == nil {
			t.Fatal("Extract() succeeded")
		}
		assertPathDoesNotExist(t, filepath.Join(outputDir, "secret.txt"))
	})
}

func TestExtractWritesSafeSymlinks(t *testing.T) {
	t.Run("TAR", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "bundle.tar")
		writeTARFixture(t, archivePath, false, []tarFixture{
			{Name: "libfoo.so.1.0.0", Body: "payload", Mode: 0o644, Typeflag: tar.TypeReg},
			{Name: "libfoo.so", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "libfoo.so.1.0.0"},
		})
		outputDir := filepath.Join(root, "output")

		_, err := Extract(archivePath, Options{Directory: outputDir, WorkingDir: root})

		if err != nil {
			t.Fatalf("Extract() error = %v", err)
		}
		linkPath := filepath.Join(outputDir, "libfoo.so")
		target, readErr := os.Readlink(linkPath)
		if readErr != nil || target != "libfoo.so.1.0.0" {
			t.Fatalf("Readlink(%q) = %q, %v", linkPath, target, readErr)
		}
		assertFileContents(t, linkPath, "payload")
	})

	t.Run("ZIP", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "bundle.zip")
		writeZIPFixture(t, archivePath, []zipFixture{
			{Name: "libfoo.so.1.0.0", Body: "payload", Mode: 0o644},
			{Name: "libfoo.so", Body: "libfoo.so.1.0.0", Mode: fs.ModeSymlink | 0o777},
		})
		outputDir := filepath.Join(root, "output")

		_, err := Extract(archivePath, Options{Directory: outputDir, WorkingDir: root})

		if err != nil {
			t.Fatalf("Extract() error = %v", err)
		}
		linkPath := filepath.Join(outputDir, "libfoo.so")
		target, readErr := os.Readlink(linkPath)
		if readErr != nil || target != "libfoo.so.1.0.0" {
			t.Fatalf("Readlink(%q) = %q, %v", linkPath, target, readErr)
		}
		assertFileContents(t, linkPath, "payload")
	})
}

func TestExtractReportsSkippedExistingFilesInResult(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "bundle.zip")
	writeZIPFixture(t, archivePath, []zipFixture{{Name: "note.txt", Body: "replacement"}})
	outputDir := filepath.Join(root, "output")
	if err := os.Mkdir(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "note.txt"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Extract(archivePath, Options{Directory: outputDir, WorkingDir: root})

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Skipped) != 1 || result.Skipped[0] != "note.txt" {
		t.Fatalf("Skipped = %#v", result.Skipped)
	}
	assertFileContents(t, filepath.Join(outputDir, "note.txt"), "original")
}

func TestExtractReportsSkippedFilesInResultOnLaterFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory permission this test relies on to force a write failure")
	}
	root := t.TempDir()
	archivePath := filepath.Join(root, "bundle.zip")
	writeZIPFixture(t, archivePath, []zipFixture{
		{Name: "first.txt", Body: "replacement"},
		{Name: "blocked/second.txt", Body: "cannot be written"},
	})
	outputDir := filepath.Join(root, "output")
	if err := os.Mkdir(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "first.txt"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	// "blocked" is pre-created as a read-only (but otherwise valid) directory: planning must
	// succeed (path resolution only needs to traverse it, which requires no write access), and
	// the write phase must fail when it tries to create "second.txt" inside it. This replaces
	// the original CLI-level test's file/directory type collision, which can no longer be
	// reproduced here: Extract validates every entry's destination during planning, before any
	// entry is written, so a blocking regular file placed "up front" is now caught during
	// planning (before any writes, so nothing is ever reported skipped) rather than during a
	// later write step.
	if err := os.Mkdir(filepath.Join(outputDir, "blocked"), 0o500); err != nil {
		t.Fatal(err)
	}

	result, err := Extract(archivePath, Options{Directory: outputDir, WorkingDir: root})

	if err == nil {
		t.Fatal("Extract() succeeded")
	}
	assertFileContents(t, filepath.Join(outputDir, "first.txt"), "original")
	found := false
	for _, name := range result.Skipped {
		if name == "first.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Skipped = %#v, want it to contain %q", result.Skipped, "first.txt")
	}
}

func assertFileContents(t *testing.T, path string, want string) {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != want {
		t.Fatalf("contents of %q = %q, %v; want %q", path, contents, err, want)
	}
}

func assertPathDoesNotExist(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("path %q exists or returned unexpected error: %v", path, err)
	}
}
