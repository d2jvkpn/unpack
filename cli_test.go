package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type zipFixture struct {
	Name string
	Body string
	Mode fs.FileMode
}

func writeZIPFixture(t *testing.T, path string, entries []zipFixture) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.Name, Method: zip.Deflate}
		if entry.Mode != 0 {
			header.SetMode(entry.Mode)
		}
		entryWriter, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entryWriter, entry.Body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
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

func TestRunShowsHelpAndParsesSupportedFlags(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		status := run([]string{"--help"}, &stdout, &stderr, t.TempDir())

		if status != 0 {
			t.Fatalf("run(--help) status = %d, stderr = %q", status, stderr.String())
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
}

func TestRunRequiresAtLeastOneArchive(t *testing.T) {
	var stdout, stderr bytes.Buffer
	status := run(nil, &stdout, &stderr, t.TempDir())

	if status != 2 {
		t.Fatalf("run() status = %d, want 2", status)
	}
	wantUsage := "Usage: unpack [--cn] [--directory DIR] [--overwrite] [--version] ARCHIVE [FILE...]"
	if !strings.Contains(stderr.String(), wantUsage) {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunRejectsShortOutputAlias(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "bundle.zip")
	writeZIPFixture(t, archivePath, []zipFixture{{Name: "note.txt", Body: "content"}})
	var stdout, stderr bytes.Buffer

	status := run([]string{"-o", filepath.Join(root, "output"), archivePath}, &stdout, &stderr, root)

	if status != 2 {
		t.Fatalf("run(-o) status = %d, want 2", status)
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

			status := run(tt.args(root, archivePath), &stdout, &stderr, root)

			if status != 2 {
				t.Errorf("run() status = %d, want 2; stderr = %q", status, stderr.String())
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

	status := run(
		[]string{"--directory", "-destination", archivePath},
		&stdout,
		&stderr,
		workingDir,
	)

	if status != 0 {
		t.Fatalf("run() status = %d, stderr = %q", status, stderr.String())
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

	status := run([]string{archivePath, "-cn"}, &stdout, &stderr, workingDir)

	if status != 1 {
		t.Fatalf("run() status = %d, want 1 (\"-cn\" is a literal, unmatched selector); stderr = %q",
			status, stderr.String())
	}
	if !strings.Contains(stderr.String(), "-cn") {
		t.Fatalf("stderr = %q, want unmatched selector error mentioning -cn", stderr.String())
	}
	assertPathDoesNotExist(t, filepath.Join(workingDir, "note.txt"))
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

	status := run([]string{"--directory", outputDir, server.URL + "/bundle.zip"}, &stdout, &stderr, root)

	if status != 0 {
		t.Fatalf("run() status = %d, stderr = %q", status, stderr.String())
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

	status := run([]string{"--directory", outputDir, missingPath}, &stdout, &stderr, root)

	if status != 1 {
		t.Fatalf("run() status = %d, want 1; stderr = %q", status, stderr.String())
	}
	if !strings.Contains(stderr.String(), missingPath) {
		t.Fatalf("stderr = %q, want missing archive path %q", stderr.String(), missingPath)
	}
}

func TestRunTreatsTrailingArchivePathAsSelectorNotSecondArchive(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.zip")
	writeZIPFixture(t, firstPath, []zipFixture{{Name: "first.txt", Body: "first"}})
	secondPath := filepath.Join(root, "second.zip")
	writeZIPFixture(t, secondPath, []zipFixture{{Name: "second.txt", Body: "second"}})
	outputDir := filepath.Join(root, "output")
	var stdout, stderr bytes.Buffer

	status := run([]string{"--directory", outputDir, firstPath, secondPath}, &stdout, &stderr, root)

	if status != 1 {
		t.Fatalf("run() status = %d, want 1 (second.zip treated as a selector, not a second archive); stderr = %q",
			status, stderr.String())
	}
	assertPathDoesNotExist(t, filepath.Join(outputDir, "first.txt"))
	assertPathDoesNotExist(t, filepath.Join(outputDir, "second.txt"))
}
