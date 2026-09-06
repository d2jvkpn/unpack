package unpack

import (
	"archive/tar"
	"bytes"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunShowsHelpAndParsesSupportedFlags(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		status := Run([]string{"--help"}, &stdout, &stderr, t.TempDir())

		if status != 0 {
			t.Fatalf("Run(--help) status = %d, stderr = %q", status, stderr.String())
		}
		for _, text := range []string{
			"unpack [--cn] [--directory DIR] [--overwrite] [--version] ARCHIVE [FILE...]",
			"--cn",
			"--directory, -d DIR",
		} {
			if !strings.Contains(stderr.String(), text) {
				t.Errorf("help output %q does not contain %q", stderr.String(), text)
			}
		}
	})

	t.Run("cn and output directory", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "legacy.zip")
		rawName := string([]byte{0xd6, 0xd0, 0xce, 0xc4, '.', 't', 'x', 't'})
		writeZIPFixture(t, archivePath, []zipFixture{{Name: rawName, NonUTF8: true, Body: "decoded"}})
		outputDir := filepath.Join(root, "output")
		var stdout, stderr bytes.Buffer

		status := Run(
			[]string{"--cn", "--directory", outputDir, archivePath},
			&stdout,
			&stderr,
			filepath.Join(root, "working"),
		)

		if status != 0 {
			t.Fatalf("Run() status = %d, stderr = %q", status, stderr.String())
		}
		assertFileContents(t, filepath.Join(outputDir, "中文.txt"), "decoded")
	})
}

func TestRunRequiresAtLeastOneArchive(t *testing.T) {
	var stdout, stderr bytes.Buffer
	status := Run(nil, &stdout, &stderr, t.TempDir())

	if status != 2 {
		t.Fatalf("Run() status = %d, want 2", status)
	}
	wantUsage := "Usage: unpack [--cn] [--directory DIR] [--overwrite] [--version] ARCHIVE [FILE...]"
	if !strings.Contains(stderr.String(), wantUsage) {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunUsesSmartDefaultDestinations(t *testing.T) {
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
		var stdout, stderr bytes.Buffer

		status := Run([]string{archivePath}, &stdout, &stderr, workingDir)

		if status != 0 {
			t.Fatalf("Run() status = %d, stderr = %q", status, stderr.String())
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
		var stdout, stderr bytes.Buffer

		status := Run([]string{archivePath}, &stdout, &stderr, workingDir)

		if status != 0 {
			t.Fatalf("Run() status = %d, stderr = %q", status, stderr.String())
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
		var stdout, stderr bytes.Buffer

		status := Run([]string{archivePath}, &stdout, &stderr, workingDir)

		if status != 0 {
			t.Fatalf("Run() status = %d, stderr = %q", status, stderr.String())
		}
		assertFileContents(t, filepath.Join(workingDir, "bundle", "first.txt"), "first")
		assertFileContents(t, filepath.Join(workingDir, "bundle", "docs", "second.txt"), "second")
		assertPathDoesNotExist(t, filepath.Join(workingDir, "bundle.tar"))
	})
}

func TestRunRejectsUnsafeSyntheticDefaultBases(t *testing.T) {
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
			var stdout, stderr bytes.Buffer

			status := Run([]string{archivePath}, &stdout, &stderr, workingDir)

			if status != 1 {
				t.Fatalf("Run() status = %d, want 1; stderr = %q", status, stderr.String())
			}
			assertPathDoesNotExist(t, tt.payloadPath(root, workingDir))
		})
	}
}

func TestRunRejectsSyntheticDefaultBaseSymlinkEscape(t *testing.T) {
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
	var stdout, stderr bytes.Buffer

	status := Run([]string{archivePath}, &stdout, &stderr, workingDir)

	if status != 1 {
		t.Fatalf("Run() status = %d, want 1; stderr = %q", status, stderr.String())
	}
	assertPathDoesNotExist(t, filepath.Join(outsideDir, "first.txt"))
	assertPathDoesNotExist(t, filepath.Join(outsideDir, "docs", "second.txt"))
}

func TestRunAcceptsRootDirectoryMarkers(t *testing.T) {
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
			var stdout, stderr bytes.Buffer

			status := Run([]string{"--directory", outputDir, archivePath}, &stdout, &stderr, root)

			if status != 0 {
				t.Fatalf("Run() status = %d, stderr = %q", status, stderr.String())
			}
			assertFileContents(t, filepath.Join(outputDir, tt.payloadName), strings.TrimSuffix(tt.payloadName, ".txt"))
		})
	}
}

func TestRunExplicitOutputStripsSoleTopLevelDirectory(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "bundle.zip")
	writeZIPFixture(t, archivePath, []zipFixture{
		{Name: "wrapper/", Mode: fs.ModeDir | 0o755},
		{Name: "wrapper/note.txt", Body: "unwrapped"},
	})
	outputDir := filepath.Join(root, "output")
	var stdout, stderr bytes.Buffer

	status := Run([]string{"--directory", outputDir, archivePath}, &stdout, &stderr, root)

	if status != 0 {
		t.Fatalf("Run() status = %d, stderr = %q", status, stderr.String())
	}
	assertFileContents(t, filepath.Join(outputDir, "note.txt"), "unwrapped")
	assertPathDoesNotExist(t, filepath.Join(outputDir, "wrapper"))
}

func TestRunRejectsShortOutputAlias(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "bundle.zip")
	writeZIPFixture(t, archivePath, []zipFixture{{Name: "note.txt", Body: "content"}})
	var stdout, stderr bytes.Buffer

	status := Run([]string{"-o", filepath.Join(root, "output"), archivePath}, &stdout, &stderr, root)

	if status != 2 {
		t.Fatalf("Run(-o) status = %d, want 2", status)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -o") {
		t.Fatalf("stderr = %q, want rejected -o flag", stderr.String())
	}
	assertPathDoesNotExist(t, filepath.Join(root, "output"))
}

func TestRunRejectsSingleDashLongOptionsWithoutWriting(t *testing.T) {
	tests := []struct {
		name        string
		args        func(root string, archivePath string) []string
		writtenPath func(root string) string
	}{
		{
			name: "cn",
			args: func(_ string, archivePath string) []string {
				return []string{"-cn", archivePath}
			},
			writtenPath: func(root string) string { return filepath.Join(root, "note.txt") },
		},
		{
			name: "output directory separate value",
			args: func(root string, archivePath string) []string {
				return []string{"-directory", filepath.Join(root, "output"), archivePath}
			},
			writtenPath: func(root string) string { return filepath.Join(root, "output", "note.txt") },
		},
		{
			name: "output directory equals value",
			args: func(root string, archivePath string) []string {
				return []string{"-directory=" + filepath.Join(root, "output"), archivePath}
			},
			writtenPath: func(root string) string { return filepath.Join(root, "output", "note.txt") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			archivePath := filepath.Join(root, "bundle.zip")
			writeZIPFixture(t, archivePath, []zipFixture{{Name: "note.txt", Body: "content"}})
			var stdout, stderr bytes.Buffer

			status := Run(tt.args(root, archivePath), &stdout, &stderr, root)

			if status != 2 {
				t.Errorf("Run() status = %d, want 2; stderr = %q", status, stderr.String())
			}
			assertPathDoesNotExist(t, tt.writtenPath(root))
		})
	}
}

func TestRunAcceptsDashPrefixedOutputDirectoryOperand(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "archives", "bundle.zip")
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeZIPFixture(t, archivePath, []zipFixture{{Name: "note.txt", Body: "content"}})
	workingDir := filepath.Join(root, "working")
	if err := os.Mkdir(workingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	status := Run(
		[]string{"--directory", "-destination", archivePath},
		&stdout,
		&stderr,
		workingDir,
	)

	if status != 0 {
		t.Fatalf("Run() status = %d, stderr = %q", status, stderr.String())
	}
	assertFileContents(t, filepath.Join(workingDir, "-destination", "note.txt"), "content")
}

func TestRunStopsOptionParsingAtFirstArchive(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "archives", "bundle.zip")
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeZIPFixture(t, archivePath, []zipFixture{{Name: "note.txt", Body: "content"}})
	workingDir := filepath.Join(root, "working")
	if err := os.Mkdir(workingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	status := Run([]string{archivePath, "-cn"}, &stdout, &stderr, workingDir)

	if status != 1 {
		t.Fatalf("Run() status = %d, want 1 (\"-cn\" is a literal, unmatched selector); stderr = %q",
			status, stderr.String())
	}
	if !strings.Contains(stderr.String(), "-cn") {
		t.Fatalf("stderr = %q, want unmatched selector error mentioning -cn", stderr.String())
	}
	assertPathDoesNotExist(t, filepath.Join(workingDir, "note.txt"))
}

func TestRunExtractsBothSupportedFormats(t *testing.T) {
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
		var stdout, stderr bytes.Buffer
		status := Run([]string{"--directory", outputDir, tt.archivePath}, &stdout, &stderr, root)
		if status != 0 {
			t.Fatalf("Run() status = %d, stderr = %q", status, stderr.String())
		}
		assertFileContents(t, filepath.Join(outputDir, tt.fileName), tt.body)
	}
}

func TestRunDownloadsArchiveFromURL(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "bundle.zip")
	writeZIPFixture(t, archivePath, []zipFixture{{Name: "note.txt", Body: "downloaded"}})
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(archiveBytes); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()
	outputDir := filepath.Join(root, "output")
	var stdout, stderr bytes.Buffer

	status := Run([]string{"--directory", outputDir, server.URL + "/bundle.zip"}, &stdout, &stderr, root)

	if status != 0 {
		t.Fatalf("Run() status = %d, stderr = %q", status, stderr.String())
	}
	assertFileContents(t, filepath.Join(outputDir, "note.txt"), "downloaded")
	if !strings.Contains(stdout.String(), "Downloading: "+server.URL+"/bundle.zip") {
		t.Fatalf("stdout = %q, want download progress", stdout.String())
	}
}

func TestRunReportsErrorForMissingArchive(t *testing.T) {
	root := t.TempDir()
	missingPath := filepath.Join(root, "missing.zip")
	outputDir := filepath.Join(root, "output")
	var stdout, stderr bytes.Buffer

	status := Run([]string{"--directory", outputDir, missingPath}, &stdout, &stderr, root)

	if status != 1 {
		t.Fatalf("Run() status = %d, want 1; stderr = %q", status, stderr.String())
	}
	if !strings.Contains(stderr.String(), missingPath) {
		t.Fatalf("stderr = %q, want missing archive path %q", stderr.String(), missingPath)
	}
}

func TestRunExtractsOnlySelectedFiles(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "bundle.zip")
	writeZIPFixture(t, archivePath, []zipFixture{
		{Name: "first.txt", Body: "first"},
		{Name: "second.txt", Body: "second"},
		{Name: "docs/third.txt", Body: "third"},
	})
	outputDir := filepath.Join(root, "output")
	var stdout, stderr bytes.Buffer

	status := Run([]string{"--directory", outputDir, archivePath, "first.txt", "docs"}, &stdout, &stderr, root)

	if status != 0 {
		t.Fatalf("Run() status = %d, stderr = %q", status, stderr.String())
	}
	assertFileContents(t, filepath.Join(outputDir, "first.txt"), "first")
	assertFileContents(t, filepath.Join(outputDir, "docs", "third.txt"), "third")
	assertPathDoesNotExist(t, filepath.Join(outputDir, "second.txt"))
}

func TestRunSelectorMatchesRelativeToSoleTopLevelDirectory(t *testing.T) {
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
	var stdout, stderr bytes.Buffer

	status := Run([]string{archivePath, "src/main.go"}, &stdout, &stderr, workingDir)

	if status != 0 {
		t.Fatalf("Run() status = %d, stderr = %q", status, stderr.String())
	}
	assertFileContents(t, filepath.Join(workingDir, "wrapper", "src", "main.go"), "main")
	assertPathDoesNotExist(t, filepath.Join(workingDir, "wrapper", "README.md"))
}

func TestRunFailsWithoutWritingWhenSelectorMatchesNothing(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "bundle.zip")
	writeZIPFixture(t, archivePath, []zipFixture{{Name: "first.txt", Body: "first"}})
	outputDir := filepath.Join(root, "output")
	var stdout, stderr bytes.Buffer

	status := Run([]string{"--directory", outputDir, archivePath, "missing.txt"}, &stdout, &stderr, root)

	if status != 1 {
		t.Fatalf("Run() status = %d, want 1; stderr = %q", status, stderr.String())
	}
	if !strings.Contains(stderr.String(), "missing.txt") {
		t.Fatalf("stderr = %q, want unmatched selector reported", stderr.String())
	}
	assertPathDoesNotExist(t, filepath.Join(outputDir, "first.txt"))
}

func TestRunTreatsTrailingArchivePathAsSelectorNotSecondArchive(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.zip")
	writeZIPFixture(t, firstPath, []zipFixture{{Name: "first.txt", Body: "first"}})
	secondPath := filepath.Join(root, "second.zip")
	writeZIPFixture(t, secondPath, []zipFixture{{Name: "second.txt", Body: "second"}})
	outputDir := filepath.Join(root, "output")
	var stdout, stderr bytes.Buffer

	status := Run([]string{"--directory", outputDir, firstPath, secondPath}, &stdout, &stderr, root)

	if status != 1 {
		t.Fatalf("Run() status = %d, want 1 (second.zip treated as a selector, not a second archive); stderr = %q",
			status, stderr.String())
	}
	assertPathDoesNotExist(t, filepath.Join(outputDir, "first.txt"))
	assertPathDoesNotExist(t, filepath.Join(outputDir, "second.txt"))
}

func TestRunRejectsUnsafeArchivesBeforeWritingPayloads(t *testing.T) {
	t.Run("path traversal", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "traversal.zip")
		writeZIPFixture(t, archivePath, []zipFixture{
			{Name: "safe.txt", Body: "must not be written"},
			{Name: "../escape.txt", Body: "escape"},
		})
		outputDir := filepath.Join(root, "output")
		var stdout, stderr bytes.Buffer

		status := Run([]string{"--directory", outputDir, archivePath}, &stdout, &stderr, root)

		if status != 1 {
			t.Fatalf("Run() status = %d, want 1; stderr = %q", status, stderr.String())
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
		var stdout, stderr bytes.Buffer

		status := Run([]string{"--directory", outputDir, archivePath}, &stdout, &stderr, root)

		if status != 1 {
			t.Fatalf("Run() status = %d, want 1; stderr = %q", status, stderr.String())
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
		var stdout, stderr bytes.Buffer

		status := Run([]string{"--directory", outputDir, archivePath}, &stdout, &stderr, root)

		if status != 1 {
			t.Fatalf("Run() status = %d, want 1; stderr = %q", status, stderr.String())
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
		var stdout, stderr bytes.Buffer

		status := Run([]string{"--directory", outputDir, archivePath}, &stdout, &stderr, root)

		if status != 1 {
			t.Fatalf("Run() status = %d, want 1; stderr = %q", status, stderr.String())
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
		var stdout, stderr bytes.Buffer

		status := Run([]string{"--directory", outputDir, archivePath}, &stdout, &stderr, root)

		if status != 1 {
			t.Fatalf("Run() status = %d, want 1; stderr = %q", status, stderr.String())
		}
		assertPathDoesNotExist(t, filepath.Join(outputDir, "linked"))
	})

	t.Run("encrypted ZIP", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "encrypted.zip")
		writeZIPFixture(t, archivePath, []zipFixture{{Name: "secret.txt", Body: "secret"}})
		patchZIPEncrypted(t, archivePath)
		outputDir := filepath.Join(root, "output")
		var stdout, stderr bytes.Buffer

		status := Run([]string{"--directory", outputDir, archivePath}, &stdout, &stderr, root)

		if status != 1 {
			t.Fatalf("Run() status = %d, want 1; stderr = %q", status, stderr.String())
		}
		assertPathDoesNotExist(t, filepath.Join(outputDir, "secret.txt"))
	})
}

func TestRunExtractsSafeSymlinks(t *testing.T) {
	t.Run("TAR", func(t *testing.T) {
		root := t.TempDir()
		archivePath := filepath.Join(root, "bundle.tar")
		writeTARFixture(t, archivePath, false, []tarFixture{
			{Name: "libfoo.so.1.0.0", Body: "payload", Mode: 0o644, Typeflag: tar.TypeReg},
			{Name: "libfoo.so", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "libfoo.so.1.0.0"},
		})
		outputDir := filepath.Join(root, "output")
		var stdout, stderr bytes.Buffer

		status := Run([]string{"--directory", outputDir, archivePath}, &stdout, &stderr, root)

		if status != 0 {
			t.Fatalf("Run() status = %d, want 0; stderr = %q", status, stderr.String())
		}
		linkPath := filepath.Join(outputDir, "libfoo.so")
		target, err := os.Readlink(linkPath)
		if err != nil || target != "libfoo.so.1.0.0" {
			t.Fatalf("Readlink(%q) = %q, %v", linkPath, target, err)
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
		var stdout, stderr bytes.Buffer

		status := Run([]string{"--directory", outputDir, archivePath}, &stdout, &stderr, root)

		if status != 0 {
			t.Fatalf("Run() status = %d, want 0; stderr = %q", status, stderr.String())
		}
		linkPath := filepath.Join(outputDir, "libfoo.so")
		target, err := os.Readlink(linkPath)
		if err != nil || target != "libfoo.so.1.0.0" {
			t.Fatalf("Readlink(%q) = %q, %v", linkPath, target, err)
		}
		assertFileContents(t, linkPath, "payload")
	})
}

func TestRunReportsProgressAndPreservesExistingFiles(t *testing.T) {
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
	var stdout, stderr bytes.Buffer

	status := Run([]string{"--directory", outputDir, archivePath}, &stdout, &stderr, root)

	if status != 0 {
		t.Fatalf("Run() status = %d, stderr = %q", status, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Extracting: "+archivePath) {
		t.Fatalf("stdout = %q, want extraction progress", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Skipping existing file: note.txt") {
		t.Fatalf("stdout = %q, want skipped-file message", stdout.String())
	}
	assertFileContents(t, filepath.Join(outputDir, "note.txt"), "original")
}

func TestRunReportsSkipsBeforeLaterExtractionFailure(t *testing.T) {
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
	stdout := &triggeringWriter{trigger: func() error {
		return os.WriteFile(filepath.Join(outputDir, "blocked"), []byte("regular file"), 0o600)
	}}
	var stderr bytes.Buffer

	status := Run([]string{"--directory", outputDir, archivePath}, stdout, &stderr, root)

	if stdout.triggerErr != nil {
		t.Fatal(stdout.triggerErr)
	}
	if status != 1 {
		t.Fatalf("Run() status = %d, want 1; stderr = %q", status, stderr.String())
	}
	assertFileContents(t, filepath.Join(outputDir, "first.txt"), "original")
	if !strings.Contains(stdout.String(), "Skipping existing file: first.txt") {
		t.Fatalf("stdout = %q, want skip reported before later extraction failure", stdout.String())
	}
}

type triggeringWriter struct {
	bytes.Buffer
	trigger    func() error
	triggerErr error
}

func (w *triggeringWriter) Write(p []byte) (int, error) {
	if w.trigger != nil {
		trigger := w.trigger
		w.trigger = nil
		w.triggerErr = trigger()
	}
	return w.Buffer.Write(p)
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
