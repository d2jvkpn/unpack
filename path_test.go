package main

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestPlanEntriesSelectsDestination(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name         string
		entries      []archiveEntry
		output       string
		wantBase     string
		wantRelative string
	}{
		{"default single", []archiveEntry{{Name: "one/file.txt", Kind: entryFile}}, "",
			root, "one/file.txt"},
		{"default multiple", []archiveEntry{{Name: "a.txt", Kind: entryFile},
			{Name: "dir/b.txt", Kind: entryFile}}, "", filepath.Join(root, "bundle"), "a.txt"},
		{"explicit strips wrapper", []archiveEntry{{Name: "one/file.txt", Kind: entryFile}},
			filepath.Join(root, "out"), filepath.Join(root, "out"), "file.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plans, base, err := planEntries(
				tt.entries,
				filepath.Join(root, "bundle.tar.gz"),
				tt.output,
				root,
				formatTarGzip,
			)
			if err != nil || base != tt.wantBase || plans[0].RelativeName != tt.wantRelative {
				t.Fatalf("plan = %#v, %q, %v", plans, base, err)
			}
		})
	}
}

func TestPlanEntriesRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"../escape", "/absolute"} {
		_, _, err := planEntries(
			[]archiveEntry{{Name: name, Kind: entryFile}}, "bundle.zip", root, root, formatZIP,
		)
		if err == nil {
			t.Fatalf("unsafe name %q succeeded", name)
		}
	}
}

func TestPlanEntriesRejectsRootMarkerFile(t *testing.T) {
	root := t.TempDir()
	_, _, err := planEntries(
		[]archiveEntry{{Name: ".", Kind: entryFile}}, "bundle.zip", root, root, formatZIP,
	)
	if err == nil {
		t.Fatal("regular-file root marker succeeded")
	}
}

func TestPlanEntriesRejectsWindowsDriveAbsolute(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"C:/escape", "C:\\escape"} {
		_, _, err := planEntries(
			[]archiveEntry{{Name: name, Kind: entryFile}}, "bundle.zip", root, root, formatZIP,
		)
		if err == nil {
			t.Fatalf("unsafe Windows drive path %q succeeded", name)
		}
	}
}

func TestPlanEntriesRejectsUnknownEntryKind(t *testing.T) {
	root := t.TempDir()
	for _, kind := range []entryKind{0, entryKind(99)} {
		_, _, err := planEntries(
			[]archiveEntry{{Name: "file.txt", Kind: kind}}, "bundle.zip", root, root, formatZIP,
		)
		if err == nil {
			t.Fatalf("unknown entry kind %d succeeded", kind)
		}
	}
}

func TestPlanEntriesRejectsFileDirectoryConflict(t *testing.T) {
	root := t.TempDir()
	_, _, err := planEntries(
		[]archiveEntry{{Name: "one", Kind: entryFile}, {Name: "one/child.txt", Kind: entryFile}},
		"bundle.zip", filepath.Join(root, "out"), root, formatZIP,
	)
	if err == nil {
		t.Fatal("file entry with descendants succeeded")
	}
}

func TestPlanEntriesRejectsExistingSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "out")
	if err := os.Mkdir(base, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base, "linked")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	_, _, err := planEntries(
		[]archiveEntry{{Name: "safe.txt", Kind: entryFile}, {Name: "linked/escape.txt", Kind: entryFile}},
		"bundle.zip", base, root, formatZIP,
	)
	if err == nil {
		t.Fatal("entry through pre-existing symlink succeeded")
	}
}

func TestPlanEntriesDoesNotCreateDirectoriesBeforeValidation(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "new-output")
	_, _, err := planEntries(
		[]archiveEntry{{Name: "safe.txt", Kind: entryFile}, {Name: "../escape", Kind: entryFile}},
		"bundle.zip", base, root, formatZIP,
	)
	if err == nil {
		t.Fatal("unsafe plan succeeded")
	}
	if _, statErr := os.Stat(base); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("planEntries created base: %v", statErr)
	}
}

func TestDirectoryFinalizerUsesDefaultModeForImplicitDirectories(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "implicit")
	child := filepath.Join(parent, "nested")
	directories := newDirectoryFinalizer(false)
	if err := directories.ensure(child); err != nil {
		t.Fatal(err)
	}
	if err := directories.finalize(); err != nil {
		t.Fatal(err)
	}

	assertDirectoryMode(t, parent, 0o755)
	assertDirectoryMode(t, child, 0o755)
}

func TestWriteNewFileSkipsExistingFile(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "existing.txt")
	if err := os.WriteFile(destination, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	skipped, err := writeNewFile(destination, 0o644, false, func(io.Writer) error {
		return errors.New("write should not run")
	})
	if err != nil || !skipped {
		t.Fatalf("writeNewFile() = skipped %v, err %v", skipped, err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != "original" {
		t.Fatalf("existing file = %q, %v", got, err)
	}
}

func TestWriteNewFileSkipsFinalSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, destination); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	skipped, err := writeNewFile(destination, 0o644, false, func(io.Writer) error {
		return errors.New("write should not run")
	})
	if err != nil || !skipped {
		t.Fatalf("writeNewFile() = skipped %v, err %v", skipped, err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "original" {
		t.Fatalf("symlink target = %q, %v", got, err)
	}
}

func TestWriteNewFileRemovesPartialFileAfterWriteFailure(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "partial.txt")

	skipped, err := writeNewFile(destination, 0o640, false, func(w io.Writer) error {
		if _, writeErr := io.WriteString(w, "partial"); writeErr != nil {
			return writeErr
		}
		return errors.New("write failed")
	})
	if err == nil || skipped {
		t.Fatalf("writeNewFile() = skipped %v, err %v", skipped, err)
	}
	if _, statErr := os.Stat(destination); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("partial file remains: %v", statErr)
	}
}
