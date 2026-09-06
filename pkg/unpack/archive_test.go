package unpack

import (
	"errors"
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
