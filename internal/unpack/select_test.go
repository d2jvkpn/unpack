package unpack

import "testing"

func TestFilterPlansKeepsOnlyMatchedEntries(t *testing.T) {
	plans := []plannedEntry{
		{ArchiveName: "a.txt"},
		{ArchiveName: "b.txt"},
		{ArchiveName: "docs/c.txt"},
	}

	filtered, err := filterPlans(plans, []string{"a.txt", "docs"})
	if err != nil {
		t.Fatalf("filterPlans() error = %v", err)
	}
	if len(filtered) != 2 || filtered[0].ArchiveName != "a.txt" || filtered[1].ArchiveName != "docs/c.txt" {
		t.Fatalf("filterPlans() = %#v", filtered)
	}
}

func TestFilterPlansErrorsOnUnmatchedSelector(t *testing.T) {
	plans := []plannedEntry{{ArchiveName: "a.txt"}}

	_, err := filterPlans(plans, []string{"a.txt", "missing.txt"})
	if err == nil {
		t.Fatal("filterPlans() with unmatched selector succeeded")
	}
}

func TestFilterPlansUsesTopLevelDirForMatching(t *testing.T) {
	plans := []plannedEntry{
		{ArchiveName: "myproj", TopLevelDir: "myproj"},
		{ArchiveName: "myproj/src/main.go", TopLevelDir: "myproj"},
		{ArchiveName: "myproj/README.md", TopLevelDir: "myproj"},
	}

	filtered, err := filterPlans(plans, []string{"src/main.go"})
	if err != nil {
		t.Fatalf("filterPlans() error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].ArchiveName != "myproj/src/main.go" {
		t.Fatalf("filterPlans() = %#v", filtered)
	}
}

func TestSelectorMatchesExactPath(t *testing.T) {
	if !selectorMatches("src/main.go", "src/main.go") {
		t.Fatal("exact path did not match")
	}
	if selectorMatches("src/main.go", "src/other.go") {
		t.Fatal("distinct path matched")
	}
}

func TestSelectorMatchesDirectoryPrefix(t *testing.T) {
	if !selectorMatches("docs", "docs/a.txt") {
		t.Fatal("directory selector did not match direct child")
	}
	if !selectorMatches("docs", "docs/sub/b.txt") {
		t.Fatal("directory selector did not match nested descendant")
	}
	if selectorMatches("docs", "docsx/a.txt") {
		t.Fatal("directory selector matched unrelated sibling with shared prefix")
	}
}

func TestSelectorMatchesGlobCrossesDirectorySeparator(t *testing.T) {
	if !selectorMatches("*.md", "README.md") {
		t.Fatal("*.md did not match top-level file")
	}
	if !selectorMatches("*.md", "docs/sub/guide.md") {
		t.Fatal("*.md did not match nested file (glob should cross '/')")
	}
	if selectorMatches("*.md", "docs/sub/guide.txt") {
		t.Fatal("*.md matched a non-.md file")
	}
}

func TestSelectorMatchesQuestionMarkWildcard(t *testing.T) {
	if !selectorMatches("file?.txt", "file1.txt") {
		t.Fatal("file?.txt did not match file1.txt")
	}
	if selectorMatches("file?.txt", "file10.txt") {
		t.Fatal("file?.txt matched file10.txt")
	}
}

func TestSelectorMatchesCharacterClass(t *testing.T) {
	if !selectorMatches("file[12].txt", "file1.txt") {
		t.Fatal("file[12].txt did not match file1.txt")
	}
	if selectorMatches("file[12].txt", "file3.txt") {
		t.Fatal("file[12].txt matched file3.txt")
	}
}
