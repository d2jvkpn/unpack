package unpack

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
		entries      []Entry
		output       string
		wantBase     string
		wantRelative string
	}{
		{"default single", []Entry{{Name: "one/file.txt", Kind: EntryFile}}, "",
			root, "one/file.txt"},
		{"default multiple", []Entry{{Name: "a.txt", Kind: EntryFile},
			{Name: "dir/b.txt", Kind: EntryFile}}, "", filepath.Join(root, "bundle"), "a.txt"},
		{"explicit strips wrapper", []Entry{{Name: "one/file.txt", Kind: EntryFile}},
			filepath.Join(root, "out"), filepath.Join(root, "out"), "file.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plans, base, err := PlanEntries(
				tt.entries,
				filepath.Join(root, "bundle.tar.gz"),
				tt.output,
				root,
				FormatTarGzip,
			)
			if err != nil || base != tt.wantBase || plans[0].RelativeName != tt.wantRelative {
				t.Fatalf("plan = %#v, %q, %v", plans, base, err)
			}
		})
	}
}

func TestPlanEntriesTracksTopLevelDirIndependentlyOfOnDiskStripping(t *testing.T) {
	root := t.TempDir()
	entries := []Entry{
		{Name: "myproj/src/main.go", Kind: EntryFile},
		{Name: "myproj/README.md", Kind: EntryFile},
	}

	t.Run("no output dir: on-disk path keeps wrapper, match name does not", func(t *testing.T) {
		plans, _, err := PlanEntries(entries, filepath.Join(root, "bundle.zip"), "", root, FormatZIP)
		if err != nil {
			t.Fatalf("PlanEntries() error = %v", err)
		}
		if plans[0].ArchiveName != "myproj/src/main.go" || plans[0].TopLevelDir != "myproj" {
			t.Fatalf("plans[0] = %#v", plans[0])
		}
		if plans[0].RelativeName != filepath.FromSlash("myproj/src/main.go") {
			t.Fatalf("RelativeName = %q, want wrapper kept on disk", plans[0].RelativeName)
		}
	})

	t.Run("explicit output dir: on-disk path strips wrapper, match name unaffected", func(t *testing.T) {
		output := filepath.Join(root, "out")
		plans, _, err := PlanEntries(entries, filepath.Join(root, "bundle.zip"), output, root, FormatZIP)
		if err != nil {
			t.Fatalf("PlanEntries() error = %v", err)
		}
		if plans[0].ArchiveName != "myproj/src/main.go" || plans[0].TopLevelDir != "myproj" {
			t.Fatalf("plans[0] = %#v", plans[0])
		}
		if plans[0].RelativeName != filepath.FromSlash("src/main.go") {
			t.Fatalf("RelativeName = %q, want wrapper stripped on disk", plans[0].RelativeName)
		}
	})
}

func TestPlanEntriesLeavesTopLevelDirEmptyForMultipleTopLevelEntries(t *testing.T) {
	root := t.TempDir()
	entries := []Entry{
		{Name: "a.txt", Kind: EntryFile},
		{Name: "dir/b.txt", Kind: EntryFile},
	}

	plans, _, err := PlanEntries(entries, filepath.Join(root, "bundle.zip"), "", root, FormatZIP)
	if err != nil {
		t.Fatalf("PlanEntries() error = %v", err)
	}
	for _, plan := range plans {
		if plan.TopLevelDir != "" {
			t.Fatalf("plan %#v: want empty TopLevelDir for multi-top-level archive", plan)
		}
	}
}

func TestPlanEntriesRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"../escape", "/absolute"} {
		_, _, err := PlanEntries(
			[]Entry{{Name: name, Kind: EntryFile}}, "bundle.zip", root, root, FormatZIP,
		)
		if err == nil {
			t.Fatalf("unsafe name %q succeeded", name)
		}
		if !errors.Is(err, ErrUnsafeEntry) {
			t.Fatalf("PlanEntries(%q) error = %v, want ErrUnsafeEntry", name, err)
		}
	}
}

func TestPlanEntriesRejectsRootMarkerFile(t *testing.T) {
	root := t.TempDir()
	_, _, err := PlanEntries(
		[]Entry{{Name: ".", Kind: EntryFile}}, "bundle.zip", root, root, FormatZIP,
	)
	if err == nil {
		t.Fatal("regular-file root marker succeeded")
	}
}

func TestPlanEntriesRejectsWindowsDriveAbsolute(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"C:/escape", "C:\\escape"} {
		_, _, err := PlanEntries(
			[]Entry{{Name: name, Kind: EntryFile}}, "bundle.zip", root, root, FormatZIP,
		)
		if err == nil {
			t.Fatalf("unsafe Windows drive path %q succeeded", name)
		}
	}
}

func TestPlanEntriesRejectsUnknownEntryKind(t *testing.T) {
	root := t.TempDir()
	for _, kind := range []EntryKind{0, EntryKind(99)} {
		_, _, err := PlanEntries(
			[]Entry{{Name: "file.txt", Kind: kind}}, "bundle.zip", root, root, FormatZIP,
		)
		if err == nil {
			t.Fatalf("unknown entry kind %d succeeded", kind)
		}
	}
}

func TestPlanEntriesRejectsFileDirectoryConflict(t *testing.T) {
	root := t.TempDir()
	_, _, err := PlanEntries(
		[]Entry{{Name: "one", Kind: EntryFile}, {Name: "one/child.txt", Kind: EntryFile}},
		"bundle.zip", filepath.Join(root, "out"), root, FormatZIP,
	)
	if err == nil {
		t.Fatal("file entry with descendants succeeded")
	}
}

func TestPlanEntriesAcceptsSafeSymlink(t *testing.T) {
	root := t.TempDir()
	plans, _, err := PlanEntries(
		[]Entry{
			{Name: "lib/libfoo.so.1", Kind: EntryFile},
			{Name: "lib/libfoo.so", Kind: EntrySymlink, LinkTarget: "libfoo.so.1"},
		},
		"bundle.zip", filepath.Join(root, "out"), root, FormatZIP,
	)
	if err != nil {
		t.Fatalf("PlanEntries() error = %v", err)
	}
	if plans[1].Entry.Kind != EntrySymlink || plans[1].Entry.LinkTarget != "libfoo.so.1" {
		t.Fatalf("plans[1] = %#v", plans[1])
	}
}

func TestPlanEntriesRejectsSymlinkTargetEscape(t *testing.T) {
	root := t.TempDir()
	for _, target := range []string{"../../etc/passwd", "/etc/passwd", ""} {
		_, _, err := PlanEntries(
			[]Entry{{Name: "linked", Kind: EntrySymlink, LinkTarget: target}},
			"bundle.zip", filepath.Join(root, "out"), root, FormatZIP,
		)
		if err == nil {
			t.Fatalf("unsafe symlink target %q succeeded", target)
		}
		if !errors.Is(err, ErrUnsafeEntry) {
			t.Fatalf("PlanEntries(%q) error = %v, want ErrUnsafeEntry", target, err)
		}
	}
}

func TestPlanEntriesRejectsSymlinkAncestor(t *testing.T) {
	root := t.TempDir()
	_, _, err := PlanEntries(
		[]Entry{
			{Name: "linked", Kind: EntrySymlink, LinkTarget: "outside"},
			{Name: "linked/escape.txt", Kind: EntryFile},
		},
		"bundle.zip", filepath.Join(root, "out"), root, FormatZIP,
	)
	if err == nil {
		t.Fatal("entry nested under a symlink succeeded")
	}
	if !errors.Is(err, ErrUnsafeEntry) {
		t.Fatalf("PlanEntries() error = %v, want ErrUnsafeEntry", err)
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

	_, _, err := PlanEntries(
		[]Entry{{Name: "safe.txt", Kind: EntryFile}, {Name: "linked/escape.txt", Kind: EntryFile}},
		"bundle.zip", base, root, FormatZIP,
	)
	if err == nil {
		t.Fatal("entry through pre-existing symlink succeeded")
	}
	if !errors.Is(err, ErrUnsafeEntry) {
		t.Fatalf("PlanEntries() error = %v, want ErrUnsafeEntry", err)
	}
}

func TestPlanEntriesDoesNotCreateDirectoriesBeforeValidation(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "new-output")
	_, _, err := PlanEntries(
		[]Entry{{Name: "safe.txt", Kind: EntryFile}, {Name: "../escape", Kind: EntryFile}},
		"bundle.zip", base, root, FormatZIP,
	)
	if err == nil {
		t.Fatal("unsafe plan succeeded")
	}
	if _, statErr := os.Stat(base); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("PlanEntries created base: %v", statErr)
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

func TestWriteSymlinkCreatesLink(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "linked")

	skipped, err := writeSymlink(destination, "target.txt", false)
	if err != nil || skipped {
		t.Fatalf("writeSymlink() = skipped %v, err %v", skipped, err)
	}
	got, err := os.Readlink(destination)
	if err != nil || got != "target.txt" {
		t.Fatalf("Readlink() = %q, %v", got, err)
	}
}

func TestWriteSymlinkSkipsExistingEntryWithoutOverwrite(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(destination, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	skipped, err := writeSymlink(destination, "target.txt", false)
	if err != nil || !skipped {
		t.Fatalf("writeSymlink() = skipped %v, err %v", skipped, err)
	}
	assertFileContents(t, destination, "original")
}

func TestWriteSymlinkReplacesExistingEntryWithOverwrite(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(destination, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	skipped, err := writeSymlink(destination, "target.txt", true)
	if err != nil || skipped {
		t.Fatalf("writeSymlink() = skipped %v, err %v", skipped, err)
	}
	got, err := os.Readlink(destination)
	if err != nil || got != "target.txt" {
		t.Fatalf("Readlink() = %q, %v", got, err)
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
