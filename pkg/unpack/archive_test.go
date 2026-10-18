package unpack

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectFormatAndBaseName(t *testing.T) {
	tests := []struct {
		path   string
		format Format
		base   string
	}{
		{"a.zip", FormatZIP, "a"},
		{"a.TAR", FormatTAR, "a"},
		{"a.tar.gz", FormatTarGzip, "a"},
		{"a.TGZ", FormatTarGzip, "a"},
	}
	for _, tt := range tests {
		got, err := DetectFormat(tt.path)
		if err != nil || got != tt.format {
			t.Fatalf("DetectFormat(%q) = %v, %v", tt.path, got, err)
		}
		if base := archiveBaseName(tt.path, got); base != tt.base {
			t.Fatalf("archiveBaseName(%q) = %q, want %q", tt.path, base, tt.base)
		}
	}
}

func TestDetectFormatRejectsUnsupportedExtension(t *testing.T) {
	_, err := DetectFormat("a.rar")
	if err == nil {
		t.Fatal("DetectFormat(a.rar) succeeded")
	}
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("DetectFormat(a.rar) error = %v, want ErrUnsupportedFormat", err)
	}
}

func copyFile(t *testing.T, from string, to string) {
	t.Helper()

	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDetectFormatSniffsContentIgnoringExtension(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "src.zip")
	tarPath := filepath.Join(dir, "src.tar")
	tgzPath := filepath.Join(dir, "src.tgz")
	writeZIPFixture(t, zipPath, []zipFixture{{Name: "a.txt", Body: "a"}})
	writeTARFixture(t, tarPath, false, []tarFixture{{Name: "a.txt", Body: "a", Mode: 0o644, Typeflag: tar.TypeReg}})
	writeTARFixture(t, tgzPath, true, []tarFixture{{Name: "a.txt", Body: "a", Mode: 0o644, Typeflag: tar.TypeReg}})

	tests := []struct {
		src    string
		target string
		want   Format
	}{
		{zipPath, "noext", FormatZIP},
		{zipPath, "wrong.tar", FormatZIP},
		{tarPath, "noext", FormatTAR},
		{tarPath, "wrong.zip", FormatTAR},
		{tgzPath, "data.bin", FormatTarGzip},
		{tgzPath, "wrong.zip", FormatTarGzip},
	}
	for _, tt := range tests {
		target := filepath.Join(dir, tt.target)
		copyFile(t, tt.src, target)
		got, err := DetectFormat(target)
		if err != nil || got != tt.want {
			t.Fatalf("DetectFormat(%s as %s) = %v, %v; want %v", filepath.Base(tt.src), tt.target, got, err, tt.want)
		}
		os.Remove(target)
	}
}

func TestDetectFormatRecognizesEmptyZIP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(path, []byte("PK\x05\x06"+strings.Repeat("\x00", 18)), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := DetectFormat(path)

	if err != nil || got != FormatZIP {
		t.Fatalf("DetectFormat(empty zip) = %v, %v", got, err)
	}
}

func TestDetectFormatRecognizesV7TARWithoutUstarMagic(t *testing.T) {
	block := make([]byte, 512)
	copy(block, "a.txt")
	copy(block[100:], "0000644\x00")
	copy(block[124:], "00000000000\x00")
	copy(block[136:], "00000000000\x00")
	copy(block[148:], "        ")
	block[156] = '0'
	sum := 0
	for _, b := range block {
		sum += int(b)
	}
	copy(block[148:], fmt.Sprintf("%06o\x00 ", sum))
	path := filepath.Join(t.TempDir(), "v7")
	if err := os.WriteFile(path, append(block, make([]byte, 1024)...), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := DetectFormat(path)

	if err != nil || got != FormatTAR {
		t.Fatalf("DetectFormat(v7 tar) = %v, %v", got, err)
	}
}

func TestDetectFormatRejectsNonArchiveContent(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.bin")
	if err := os.WriteFile(plain, []byte(strings.Repeat("hello world\n", 100)), 0o600); err != nil {
		t.Fatal(err)
	}
	gz := filepath.Join(dir, "single.gz")
	file, err := os.Create(gz)
	if err != nil {
		t.Fatal(err)
	}
	writer := gzip.NewWriter(file)
	io.WriteString(writer, strings.Repeat("hello world\n", 100))
	writer.Close()
	file.Close()
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{plain, gz, empty} {
		_, err := DetectFormat(path)
		if !errors.Is(err, ErrUnsupportedFormat) {
			t.Fatalf("DetectFormat(%s) error = %v, want ErrUnsupportedFormat", filepath.Base(path), err)
		}
	}
}

func TestDetectFormatFallsBackToSuffixWhenContentIsUnrecognized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.zip")
	if err := os.WriteFile(path, []byte("not an archive"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := DetectFormat(path)

	if err != nil || got != FormatZIP {
		t.Fatalf("DetectFormat(broken.zip) = %v, %v; want suffix fallback to ZIP", got, err)
	}
}

func TestArchiveBaseNameKeepsNameWithoutMatchingSuffix(t *testing.T) {
	tests := []struct {
		path   string
		format Format
		want   string
	}{
		{"data.bin", FormatZIP, "data.bin"},
		{"noext", FormatTarGzip, "noext"},
		{"x.zip", FormatTAR, "x.zip"},
		{"ab", FormatTarGzip, "ab"},
	}
	for _, tt := range tests {
		if got := archiveBaseName(tt.path, tt.format); got != tt.want {
			t.Fatalf("archiveBaseName(%q, %v) = %q, want %q", tt.path, tt.format, got, tt.want)
		}
	}
}

func TestParseFormat(t *testing.T) {
	tests := map[string]Format{
		"zip": FormatZIP, "tar": FormatTAR, "tar.gz": FormatTarGzip, "tgz": FormatTarGzip, "ZIP": FormatZIP,
	}
	for in, want := range tests {
		if got, err := ParseFormat(in); err != nil || got != want {
			t.Fatalf("ParseFormat(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "rar", "gz"} {
		if _, err := ParseFormat(in); !errors.Is(err, ErrUnsupportedFormat) {
			t.Fatalf("ParseFormat(%q) error = %v, want ErrUnsupportedFormat", in, err)
		}
	}
}
