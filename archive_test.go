package main

import "testing"

func TestDetectFormatAndBaseName(t *testing.T) {
	tests := []struct {
		path   string
		format archiveFormat
		base   string
	}{
		{"a.zip", formatZIP, "a"},
		{"a.TAR", formatTAR, "a"},
		{"a.tar.gz", formatTarGzip, "a"},
		{"a.TGZ", formatTarGzip, "a"},
	}
	for _, tt := range tests {
		got, err := detectFormat(tt.path)
		if err != nil || got != tt.format {
			t.Fatalf("detectFormat(%q) = %v, %v", tt.path, got, err)
		}
		if base := archiveBaseName(tt.path, got); base != tt.base {
			t.Fatalf("archiveBaseName(%q) = %q, want %q", tt.path, base, tt.base)
		}
	}
}

func TestDetectFormatRejectsUnsupportedExtension(t *testing.T) {
	if _, err := detectFormat("a.rar"); err == nil {
		t.Fatal("detectFormat(a.rar) succeeded")
	}
}
