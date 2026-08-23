package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"io/fs"
	"os"
	"testing"
)

type zipFixture struct {
	Name    string
	NonUTF8 bool
	Flags   uint16
	Body    string
	Mode    fs.FileMode
}

type tarFixture struct {
	Name     string
	Body     string
	Mode     fs.FileMode
	Typeflag byte
	Linkname string
}

func writeTARFixture(t *testing.T, path string, compressed bool, entries []tarFixture) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	var writer io.Writer = file
	var gzipWriter *gzip.Writer
	if compressed {
		gzipWriter = gzip.NewWriter(file)
		writer = gzipWriter
	}
	tarWriter := tar.NewWriter(writer)
	for _, entry := range entries {
		header := &tar.Header{
			Name:     entry.Name,
			Mode:     int64(entry.Mode.Perm()),
			Size:     int64(len(entry.Body)),
			Typeflag: entry.Typeflag,
			Linkname: entry.Linkname,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tarWriter, entry.Body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if gzipWriter != nil {
		if err := gzipWriter.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func corruptTARGzipChecksum(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 8 {
		t.Fatal("gzip footer is missing")
	}
	data[len(data)-8] ^= 0xff
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func truncateTARGzipFooter(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 4 {
		t.Fatal("gzip footer is too short to truncate")
	}
	if err := os.WriteFile(path, data[:len(data)-4], 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeZIPFixture(t *testing.T, path string, entries []zipFixture) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{
			Name:    entry.Name,
			Method:  zip.Deflate,
			NonUTF8: entry.NonUTF8,
			Flags:   entry.Flags,
		}
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

func patchZIPEncrypted(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	patchZIPFlag(t, data, []byte("PK\x03\x04"), 6)
	patchZIPFlag(t, data, []byte("PK\x01\x02"), 8)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func patchZIPFlag(t *testing.T, data []byte, signature []byte, flagOffset int) {
	t.Helper()

	offset := bytes.Index(data, signature)
	if offset < 0 || offset+flagOffset+2 > len(data) {
		t.Fatalf("ZIP header %q not found", signature)
	}
	flags := binary.LittleEndian.Uint16(data[offset+flagOffset:])
	binary.LittleEndian.PutUint16(data[offset+flagOffset:], flags|0x0001)
}
