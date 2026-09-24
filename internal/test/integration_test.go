package test

import (
	"bytes"
	"encoding/json"
	"github.com/stricttools/testisolation/go/hygiene"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/smm-h/dirstat/internal/testutil"
)

var dirstatBinary string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "dirstat-test-bin-*")
	if err != nil {
		panic("creating temp dir for binary: " + err.Error())
	}

	dirstatBinary = filepath.Join(tmpDir, "dirstat-test")
	if runtime.GOOS == "windows" {
		dirstatBinary += ".exe"
	}

	// Find the project root (two levels up from internal/test/).
	_, thisFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))

	cmd := exec.Command("go", "build", "-o", dirstatBinary, ".")
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("building dirstat binary: " + err.Error())
	}

	exitCode := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(exitCode)
}

// runDirstat executes the dirstat binary in an isolated environment: HOME
// points at an empty directory (so the user's global core.excludesFile
// cannot leak in) and COLORFGBG is cleared. Output goes to pipes, so colors
// are auto-disabled and the width fallback of 80 applies.
func runDirstat(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	cmd := exec.Command(dirstatBinary, args...)
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "HOME=") || strings.HasPrefix(e, "COLORFGBG=") {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = append(env, "HOME="+t.TempDir())

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("running dirstat: %v", err)
		}
	}
	return stdout, stderr, exitCode
}

// scanJSON runs dirstat in machine mode and returns the scan document, which
// is the envelope's payload member. Callers pass --json among their args.
func scanJSON(t *testing.T, args ...string) map[string]interface{} {
	t.Helper()
	stdout, stderr, code := runDirstat(t, args...)
	if code != 0 {
		t.Fatalf("dirstat exited %d: %s", code, stderr)
	}
	var env map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	payload, ok := env["payload"].(map[string]interface{})
	if !ok {
		t.Fatalf("envelope carries no payload object: %s", stdout)
	}
	return payload
}

func summaryField(t *testing.T, parsed map[string]interface{}, key string) int {
	t.Helper()
	summary, ok := parsed["summary"].(map[string]interface{})
	if !ok {
		t.Fatalf("no summary in %v", parsed)
	}
	v, ok := summary[key].(float64)
	if !ok {
		t.Fatalf("summary[%s] missing or not a number: %v", key, summary)
	}
	return int(v)
}

func groupFormats(t *testing.T, parsed map[string]interface{}) []string {
	t.Helper()
	raw, ok := parsed["groups"].([]interface{})
	if !ok {
		t.Fatalf("no groups in %v", parsed)
	}
	var out []string
	for _, g := range raw {
		out = append(out, g.(map[string]interface{})["format"].(string))
	}
	return out
}

func TestScanBasicTree(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"main.go":        "package main\n",
		"lib/util.go":    "package lib\n\nfunc F() {}\n",
		"docs/readme.md": "# hi\n",
		"image.png":      "\x89PNG\r\n\x1a\n",
		"emptydir/":      "",
	})

	parsed := scanJSON(t, "scan", root, "--json")
	if got := summaryField(t, parsed, "directories"); got != 4 {
		t.Errorf("directories = %d, want 4", got)
	}
	if got := summaryField(t, parsed, "files"); got != 4 {
		t.Errorf("files = %d, want 4", got)
	}
	if got := summaryField(t, parsed, "max_depth"); got != 1 {
		t.Errorf("max_depth = %d, want 1", got)
	}
	formats := groupFormats(t, parsed)
	if len(formats) != 3 { // go, md, png
		t.Errorf("unexpected formats: %v", formats)
	}
}

func TestNestedGitignore(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		".git/":           "",
		".gitignore":      "*.log\nbuild/\n",
		"keep.go":         "package keep\n",
		"trace.log":       "x\n",
		"build/out.bin":   "\x00\x01",
		"build/deep/a.md": "# ignored via parent\n",
		"sub/.gitignore":  "secret.txt\n",
		"sub/secret.txt":  "hidden\n",
		"sub/open.txt":    "visible\n",
	})

	// This test isolates gitignore semantics: --exclude "" replaces the
	// curated default excludes (which would otherwise prune build/) with a
	// list matching nothing (R45).

	// exclude: drop ignored paths, including via nested .gitignore.
	parsed := scanJSON(t, "scan", root, "--json", "--exclude", "", "--ignored", "exclude", "--hidden", "exclude")
	if got := summaryField(t, parsed, "files"); got != 2 { // keep.go, sub/open.txt
		t.Errorf("exclude: files = %d, want 2", got)
	}

	// only: keep only ignored paths; files under ignored dirs count as ignored.
	parsed = scanJSON(t, "scan", root, "--json", "--exclude", "", "--ignored", "only", "--hidden", "exclude")
	if got := summaryField(t, parsed, "files"); got != 4 { // trace.log, build/out.bin, build/deep/a.md, sub/secret.txt
		t.Errorf("only: files = %d, want 4", got)
	}

	// include: no matching at all (also picks up .git and .gitignore contents).
	parsed = scanJSON(t, "scan", root, "--json", "--exclude", "", "--ignored", "include", "--hidden", "exclude")
	if got := summaryField(t, parsed, "files"); got != 6 {
		t.Errorf("include: files = %d, want 6", got)
	}
}

func TestGitInfoExclude(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		".git/info/exclude": "*.tmp\n",
		"a.txt":             "x\n",
		"b.tmp":             "x\n",
	})
	parsed := scanJSON(t, "scan", root, "--json", "--ignored", "exclude", "--hidden", "exclude")
	if got := summaryField(t, parsed, "files"); got != 1 {
		t.Errorf("files = %d, want 1 (b.tmp excluded via .git/info/exclude)", got)
	}
}

func TestHiddenFiles(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"visible.txt":       "x\n",
		".hidden.txt":       "x\n",
		".hiddendir/in.txt": "x\n",
	})
	parsed := scanJSON(t, "scan", root, "--json", "--hidden", "include")
	if got := summaryField(t, parsed, "files"); got != 3 {
		t.Errorf("hidden include: files = %d, want 3", got)
	}
	parsed = scanJSON(t, "scan", root, "--json", "--hidden", "exclude")
	if got := summaryField(t, parsed, "files"); got != 1 {
		t.Errorf("hidden exclude: files = %d, want 1", got)
	}
	if got := summaryField(t, parsed, "directories"); got != 1 {
		t.Errorf("hidden exclude: directories = %d, want 1", got)
	}
}

func TestSymlinks(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"real.txt":      "content\n",
		"dir/inner.txt": "content\n",
	})
	if err := os.Symlink(filepath.Join(root, "real.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("cannot create symlinks: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "dir"), filepath.Join(root, "dirlink")); err != nil {
		t.Fatal(err)
	}
	// Even a symlink that name-filters would drop must be counted.
	if err := os.Symlink(filepath.Join(root, "real.txt"), filepath.Join(root, ".hidden-link")); err != nil {
		t.Fatal(err)
	}

	parsed := scanJSON(t, "scan", root, "--json", "--hidden", "exclude")
	if got := summaryField(t, parsed, "symlinks"); got != 3 {
		t.Errorf("symlinks = %d, want 3", got)
	}
	// Symlinked dir must not be followed: inner.txt counted once.
	if got := summaryField(t, parsed, "files"); got != 2 {
		t.Errorf("files = %d, want 2", got)
	}
}

// findGroup returns the group object with the given format name.
func findGroup(t *testing.T, parsed map[string]interface{}, format string) map[string]interface{} {
	t.Helper()
	raw, ok := parsed["groups"].([]interface{})
	if !ok {
		t.Fatalf("no groups in %v", parsed)
	}
	for _, g := range raw {
		m := g.(map[string]interface{})
		if m["format"].(string) == format {
			return m
		}
	}
	t.Fatalf("no group %q in %v", format, groupFormats(t, parsed))
	return nil
}

func TestUnmappedTextAndEmptyFilesCountAsText(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	// End-to-end pin for the two classification rules, against the real
	// embedded extension list: a text file whose extension is deliberately
	// absent from that list (`lock` — TOML text here, binary elsewhere) is
	// sniffed and counts its lines (R13), a Godot script counts its lines,
	// and a zero-byte file is text with zero lines (R14). Binary content
	// with an unknown extension stays binary.
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"uv.lock":     "version = 1\nrequires-python = \">=3.11\"\n",
		"player.gd":   "extends Node\n\nfunc _ready():\n\tprint(\"hi\")\n",
		"empty.png":   "",
		"asset.weird": "\x00\x01\x02\xff\xfe\x00",
	})
	parsed := scanJSON(t, "scan", root, "--json")

	for _, tc := range []struct {
		format  string
		wantLOC float64
	}{
		{"lock", 2},
		{"gd", 4},
		{"png", 0},
	} {
		g := findGroup(t, parsed, tc.format)
		if g["text"] != true {
			t.Errorf("group %q: text = %v, want true", tc.format, g["text"])
		}
		if got := g["total_loc"]; got != tc.wantLOC {
			t.Errorf("group %q: total_loc = %v, want %v", tc.format, got, tc.wantLOC)
		}
	}
	if g := findGroup(t, parsed, "weird"); g["text"] != false {
		t.Errorf("binary content with an unknown extension classified as text: %v", g)
	}
}

func TestExtensionlessFiles(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"Makefile":    "all:\n\techo hi\n",
		"sub/LICENSE": "MIT-ish text\n",
		"data.bin":    "\x00\x01\x02",
		"normal.txt":  "x\n",
	})
	parsed := scanJSON(t, "scan", root, "--json", "--list-no-ext")
	if got := summaryField(t, parsed, "files_without_extension"); got != 2 {
		t.Errorf("files_without_extension = %d, want 2", got)
	}
	// Two extensionless files plus data.bin, whose extension misses the
	// text-extensions list and is therefore sniffed too (R13).
	if got := summaryField(t, parsed, "files_sniffed"); got != 3 {
		t.Errorf("files_sniffed = %d, want 3 in hybrid mode", got)
	}
	noExt := parsed["no_extension_files"].([]interface{})
	if len(noExt) != 2 || noExt[0] != "Makefile" || noExt[1] != "sub/LICENSE" {
		t.Errorf("no_extension_files = %v, want [Makefile sub/LICENSE]", noExt)
	}

	// Extensionless text files sniff as text/plain and group by MIME type.
	found := false
	for _, f := range groupFormats(t, parsed) {
		if f == "text/plain" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected text/plain group, got %v", groupFormats(t, parsed))
	}

	// Table mode renders the section.
	stdout, _, code := runDirstat(t, "scan", root, "--list-no-ext")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "Makefile") || !strings.Contains(stdout, "sub/LICENSE") {
		t.Errorf("table output missing extensionless list:\n%s", stdout)
	}
}

func TestEmptyDirsAndEmptyResult(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"a/":   "",
		"a/b/": "",
		"c/":   "",
	})
	parsed := scanJSON(t, "scan", root, "--json")
	if got := summaryField(t, parsed, "directories"); got != 4 {
		t.Errorf("directories = %d, want 4", got)
	}
	if got := summaryField(t, parsed, "files"); got != 0 {
		t.Errorf("files = %d, want 0", got)
	}
	if groups := parsed["groups"].([]interface{}); len(groups) != 0 {
		t.Errorf("groups = %v, want empty", groups)
	}

	stdout, _, code := runDirstat(t, "scan", root)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "No files found.") {
		t.Errorf("expected 'No files found.':\n%s", stdout)
	}
}

func TestPermissionDenied(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced the same way on windows")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root; permission checks are bypassed")
	}
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"ok.txt":        "fine\n",
		"locked/in.txt": "hidden\n",
		"noread.txt":    "cannot read\n",
	})
	testutil.Chmod(t, root, "locked", 0o000)
	testutil.Chmod(t, root, "noread.txt", 0o000)

	parsed := scanJSON(t, "scan", root, "--json")
	// locked/ is unreadable (1) and noread.txt fails LOC reading (1).
	if got := summaryField(t, parsed, "unreadable"); got != 2 {
		t.Errorf("unreadable = %d, want 2", got)
	}
	if got := summaryField(t, parsed, "files"); got != 1 {
		t.Errorf("files = %d, want 1", got)
	}
	_, stderr, code := runDirstat(t, "scan", root)
	if code != 0 {
		t.Errorf("permission errors must not fail the scan: exit %d", code)
	}
	if strings.Contains(stderr, "permission") {
		t.Errorf("no warning spew expected on stderr: %q", stderr)
	}
}

func TestDepthAndExclude(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"a.txt":             "x\n",
		"d1/b.txt":          "x\n",
		"d1/d2/c.txt":       "x\n",
		"node_modules/x.js": "x\n",
	})
	parsed := scanJSON(t, "scan", root, "--json", "--depth", "1", "--exclude", "node_modules")
	if got := summaryField(t, parsed, "files"); got != 2 {
		t.Errorf("files = %d, want 2", got)
	}
	if got := summaryField(t, parsed, "directories"); got != 2 {
		t.Errorf("directories = %d, want 2", got)
	}
}

func TestTypeFilterAndSort(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"a.go":  "package a\n",
		"b.go":  "package b\n",
		"c.md":  "# c\n",
		"x.png": "\x89PNG\r\n\x1a\n",
	})
	parsed := scanJSON(t, "scan", root, "--json", "--type", "text",
		"--sort-by", "format", "--sort-order", "asc")
	formats := groupFormats(t, parsed)
	if strings.Join(formats, ",") != "go,md" {
		t.Errorf("formats = %v, want [go md]", formats)
	}
	parsed = scanJSON(t, "scan", root, "--json", "--type", "binary")
	formats = groupFormats(t, parsed)
	if strings.Join(formats, ",") != "png" {
		t.Errorf("formats = %v, want [png]", formats)
	}
}

func TestTopAndSingletonsAreTableOnly(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"a.go": "package a\n", "b.go": "package b\n",
		"one.md": "# one\n", "two.txt": "two\n",
	})

	// JSON ignores --top and --singletons (R9).
	parsed := scanJSON(t, "scan", root, "--json", "--top", "1", "--singletons", "collapse")
	if got := len(groupFormats(t, parsed)); got != 3 {
		t.Errorf("JSON groups = %d, want 3 (top/singletons must not affect JSON)", got)
	}

	// Table honors both. Narrow stats keep the Format column untruncated
	// at the 80-column non-TTY width.
	stdout, _, code := runDirstat(t, "scan", root, "--top", "1", "--singletons", "collapse",
		"--show", "table", "--stats", "count", "--stats", "total-size")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "(singletons)") {
		t.Errorf("expected collapsed (singletons) row:\n%s", stdout)
	}
	if strings.Contains(stdout, " go ") {
		t.Errorf("--top 1 should keep only the (singletons) row (count 2):\n%s", stdout)
	}
}

func TestSplitModeAndStyles(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"a.go":  "package a\n",
		"x.png": "\x89PNG\r\n\x1a\n",
	})
	stdout, _, code := runDirstat(t, "scan", root, "--no-combined")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "Text Files by Format") || !strings.Contains(stdout, "Binary Files by Format") {
		t.Errorf("split mode headings missing:\n%s", stdout)
	}

	stdout, _, code = runDirstat(t, "scan", root, "--style", "ascii")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.ContainsAny(stdout, "┌│─") {
		t.Errorf("ascii style must not use box drawing:\n%s", stdout)
	}
}

func TestStatsSelectionSkipsLOC(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs enforced permission bits")
	}
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{"a.go": "package a\n"})
	// Make the file unreadable: LOC counting would fail, so if --stats
	// count truly skips the read, the file stays readable via DirEntry.
	testutil.Chmod(t, root, "a.go", 0o000)

	parsed := scanJSON(t, "scan", root, "--json", "--stats", "count", "--method", "ext")
	if got := summaryField(t, parsed, "unreadable"); got != 0 {
		t.Errorf("unreadable = %d, want 0 (LOC reads must be skipped)", got)
	}
	if got := summaryField(t, parsed, "files"); got != 1 {
		t.Errorf("files = %d, want 1", got)
	}
	groups := parsed["groups"].([]interface{})
	g := groups[0].(map[string]interface{})
	if _, present := g["total_size"]; present {
		t.Errorf("unselected total_size present: %v", g)
	}
	if _, present := g["count"]; !present {
		t.Errorf("selected count missing: %v", g)
	}
}

func TestSortByLOCForcesComputation(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"small.go": "package a\n",
		"big.md":   strings.Repeat("line\n", 100),
	})
	// Sorting by total-loc with only count selected: LOC must still be
	// computed for ordering, but not emitted.
	parsed := scanJSON(t, "scan", root, "--json",
		"--stats", "count", "--sort-by", "total-loc", "--sort-order", "desc")
	formats := groupFormats(t, parsed)
	if strings.Join(formats, ",") != "md,go" {
		t.Errorf("formats = %v, want [md go] (sorted by LOC desc)", formats)
	}
	g := parsed["groups"].([]interface{})[0].(map[string]interface{})
	if _, present := g["total_loc"]; present {
		t.Errorf("total_loc must not be emitted when unselected: %v", g)
	}
}

func TestErrorPaths(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
	}{
		{"missing dir", []string{"scan", "/nonexistent-dirstat-path"}, 2, "does not exist"},
		{"invalid stat", []string{"scan", ".", "--stats", "bogus"}, 2, "invalid stat"},
		{"invalid sort", []string{"scan", ".", "--sort-by", "bogus"}, 2, "invalid sort column"},
		{"top below -1", []string{"scan", ".", "--top", "-5"}, 2, "--top"},
	}
	for _, tc := range tests {
		_, stderr, code := runDirstat(t, tc.args...)
		if code != tc.wantCode {
			t.Errorf("%s: exit = %d, want %d (stderr: %s)", tc.name, code, tc.wantCode, stderr)
		}
		if !strings.Contains(stderr, tc.wantErr) {
			t.Errorf("%s: stderr %q missing %q", tc.name, stderr, tc.wantErr)
		}
	}

	// A file (not a directory) target is a usage error.
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{"f.txt": "x\n"})
	_, stderr, code := runDirstat(t, "scan", filepath.Join(root, "f.txt"))
	if code != 2 || !strings.Contains(stderr, "not a directory") {
		t.Errorf("file target: exit %d, stderr %q", code, stderr)
	}
}

func TestTopValidation(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{"a.go": "package a\n"})

	// -1 and 0 mean "all groups"; positive values keep the first N (R8).
	for _, top := range []string{"-1", "0", "3"} {
		stdout, stderr, code := runDirstat(t, "scan", root, "--top", top)
		if code != 0 {
			t.Errorf("--top %s: exit = %d, want 0 (stderr: %s)", top, code, stderr)
		}
		if !strings.Contains(stdout, "go") {
			t.Errorf("--top %s: table missing the go group:\n%s", top, stdout)
		}
	}

	// Values below -1 are a usage error: exit 2, error: on stderr (R8).
	_, stderr, code := runDirstat(t, "scan", root, "--top", "-5")
	if code != 2 {
		t.Errorf("--top -5: exit = %d, want 2 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "error:") || !strings.Contains(stderr, "--top") {
		t.Errorf("--top -5: stderr %q missing error: prefix or --top mention", stderr)
	}
}

func TestExecutablesCounted(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	if runtime.GOOS == "windows" {
		t.Skip("execute bits are not meaningful on windows")
	}
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"run.sh":    "#!/bin/sh\n",
		"plain.txt": "x\n",
	})
	testutil.Chmod(t, root, "run.sh", 0o755)
	parsed := scanJSON(t, "scan", root, "--json")
	if got := summaryField(t, parsed, "executables"); got != 1 {
		t.Errorf("executables = %d, want 1", got)
	}
}
