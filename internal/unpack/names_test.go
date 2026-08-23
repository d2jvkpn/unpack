package unpack

import "testing"

func TestDecodeArchiveName(t *testing.T) {
	raw := string([]byte{0xd6, 0xd0, 0xce, 0xc4, '.', 't', 'x', 't'})
	got, err := decodeArchiveName(raw, true, true)
	if err != nil || got != "中文.txt" {
		t.Fatalf("decodeArchiveName() = %q, %v", got, err)
	}

	got, err = decodeArchiveName(raw, false, true)
	if err != nil || got != raw {
		t.Fatalf("disabled decode = %q, %v", got, err)
	}

	got, err = decodeArchiveName("中文.txt", true, false)
	if err != nil || got != "中文.txt" {
		t.Fatalf("UTF-8 decode = %q, %v", got, err)
	}
}

func TestDecodeArchiveNameRejectsInvalidGBK(t *testing.T) {
	if _, err := decodeArchiveName(string([]byte{0x81}), true, true); err == nil {
		t.Fatal("invalid GBK name succeeded")
	}
}
