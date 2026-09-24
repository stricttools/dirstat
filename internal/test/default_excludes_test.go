package test

import (
	"github.com/stricttools/testisolation/go/hygiene"
	"strings"
	"testing"

	"github.com/smm-h/dirstat/internal/testutil"
)

// TestDefaultExcludes covers the curated built-in --exclude default list
// (R45): common VCS, dependency, build, cache, and IDE directories are
// skipped with no flags at all.
func TestDefaultExcludes(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		".git/config":       "[core]\n",
		"node_modules/x.js": "x\n",
		"__pycache__/a.pyc": "\x00",
		"build/out.bin":     "\x00",
		".vscode/set.json":  "{}\n",
		"a.go":              "package a\n",
		"src/b.go":          "package b\n",
	})

	// No flags: every curated name is pruned. --ignored include keeps
	// gitignore handling out of the picture.
	parsed := scanJSON(t, "scan", root, "--json", "--ignored", "include")
	if got := summaryField(t, parsed, "files"); got != 2 { // a.go, src/b.go
		t.Errorf("files = %d, want 2 (curated defaults must prune .git, node_modules, ...)", got)
	}
	if got := summaryField(t, parsed, "directories"); got != 2 { // root, src
		t.Errorf("directories = %d, want 2", got)
	}

	// Any explicit --exclude replaces the built-in list entirely: no
	// additive merging (R45).
	parsed = scanJSON(t, "scan", root, "--json", "--ignored", "include",
		"--exclude", "__pycache__")
	if got := summaryField(t, parsed, "files"); got != 6 {
		t.Errorf("files = %d, want 6 (--exclude must replace the defaults, not merge)", got)
	}

	// --exclude "" style overrides are honored too: the list becomes [""]
	// which matches nothing (R47).
	parsed = scanJSON(t, "scan", root, "--json", "--ignored", "include",
		"--exclude", "")
	if got := summaryField(t, parsed, "files"); got != 7 {
		t.Errorf(`files = %d, want 7 (--exclude "" must scan everything)`, got)
	}
}

// TestDefaultExcludesVisibleInHelp verifies the curated list appears in
// --help (R45).
func TestDefaultExcludesVisibleInHelp(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	stdout, stderr, _ := runDirstat(t, "scan", "--help")
	out := stdout + stderr
	for _, name := range []string{".git", "node_modules", "zig-out", ".vscode"} {
		if !strings.Contains(out, name) {
			t.Errorf("scan --help missing default exclude %q:\n%s", name, out)
		}
	}
}
