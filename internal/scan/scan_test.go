package scan

import (
	"github.com/stricttools/testisolation/go/hygiene"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var testTextExts = map[string]struct{}{"go": {}, "txt": {}, "md": {}}
var testTextMimes = map[string]struct{}{"application/json": {}}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func defaultOpts(root string) Options {
	return Options{
		Root:       root,
		Method:     "ext",
		Depth:      -1,
		Ignored:    IgnoredInclude,
		Hidden:     HiddenInclude,
		TypeFilter: TypeBoth,
		NeedLOC:    true,
		TextExts:   testTextExts,
		TextMimes:  testTextMimes,
	}
}

func groupByName(t *testing.T, res *Result, name string) Group {
	t.Helper()
	for _, g := range res.Groups {
		if g.Format == name {
			return g
		}
	}
	t.Fatalf("group %q not found in %v", name, res.Groups)
	return Group{}
}

func TestScanBasicStats(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	write(t, root, "a.go", "package a\n\nfunc A() {}\n") // 3 LOC
	write(t, root, "sub/b.go", "package b")              // 1 LOC (no trailing newline)
	write(t, root, "sub/c.txt", "")                      // 0 LOC (empty)
	write(t, root, "blob.bin", "\x00\x01")
	if err := os.MkdirAll(filepath.Join(root, "emptydir"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := Scan(defaultOpts(root))
	if err != nil {
		t.Fatal(err)
	}

	if res.Summary.Directories != 3 { // root + sub + emptydir
		t.Errorf("Directories = %d, want 3", res.Summary.Directories)
	}
	if res.Summary.Files != 4 {
		t.Errorf("Files = %d, want 4", res.Summary.Files)
	}
	if res.Summary.MaxDepth != 1 {
		t.Errorf("MaxDepth = %d, want 1", res.Summary.MaxDepth)
	}
	if res.Summary.UniqueFormats != 3 {
		t.Errorf("UniqueFormats = %d, want 3", res.Summary.UniqueFormats)
	}
	if res.Summary.FilesSniffed != 0 {
		t.Errorf("FilesSniffed = %d, want 0 in ext mode", res.Summary.FilesSniffed)
	}

	goGroup := groupByName(t, res, "go")
	if goGroup.Count != 2 || !goGroup.Text || !goGroup.HasLOC {
		t.Errorf("go group: %+v", goGroup)
	}
	if goGroup.TotalLOC != 4 || goGroup.MinLOC != 1 || goGroup.MaxLOC != 3 || goGroup.AvgLOC != 2 {
		t.Errorf("go group LOC: %+v", goGroup)
	}

	txtGroup := groupByName(t, res, "txt")
	if txtGroup.TotalLOC != 0 || txtGroup.MinLOC != 0 {
		t.Errorf("empty txt file should have 0 LOC: %+v", txtGroup)
	}

	binGroup := groupByName(t, res, "bin")
	if binGroup.Text || binGroup.HasLOC {
		t.Errorf("bin group should be binary without LOC: %+v", binGroup)
	}
}

func TestScanDepthLimit(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	write(t, root, "top.txt", "x\n")
	write(t, root, "d1/mid.txt", "x\n")
	write(t, root, "d1/d2/deep.txt", "x\n")

	opts := defaultOpts(root)
	opts.Depth = 1
	res, err := Scan(opts)
	if err != nil {
		t.Fatal(err)
	}
	// d2 (depth 2) is pruned: not counted, not descended.
	if res.Summary.Directories != 2 {
		t.Errorf("Directories = %d, want 2", res.Summary.Directories)
	}
	// Files in d1 (a depth-1 directory) are still counted.
	if res.Summary.Files != 2 {
		t.Errorf("Files = %d, want 2", res.Summary.Files)
	}

	opts.Depth = 0
	res, err = Scan(opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Directories != 1 || res.Summary.Files != 1 {
		t.Errorf("depth 0: dirs=%d files=%d, want 1/1", res.Summary.Directories, res.Summary.Files)
	}
}

func TestScanExcludeAndHidden(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	write(t, root, "keep.txt", "x\n")
	write(t, root, "skipme/a.txt", "x\n")
	write(t, root, "skip.txt", "x\n")
	write(t, root, ".hidden.txt", "x\n")
	write(t, root, ".hiddendir/h.txt", "x\n")

	opts := defaultOpts(root)
	opts.Exclude = []string{"skipme", "skip.txt"}
	opts.Hidden = HiddenExclude
	res, err := Scan(opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Files != 1 {
		t.Errorf("Files = %d, want 1", res.Summary.Files)
	}
	if res.Summary.Directories != 1 {
		t.Errorf("Directories = %d, want 1 (root only)", res.Summary.Directories)
	}
}

func TestScanSymlinksCountedNotFollowed(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	write(t, root, "real.txt", "x\n")
	write(t, root, "target/inner.txt", "x\n")
	if err := os.Symlink(filepath.Join(root, "target"), filepath.Join(root, "dirlink")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "real.txt"), filepath.Join(root, "filelink.txt")); err != nil {
		t.Fatal(err)
	}
	// A symlink that would be filtered by --hidden must still be counted.
	if err := os.Symlink(filepath.Join(root, "real.txt"), filepath.Join(root, ".hiddenlink")); err != nil {
		t.Fatal(err)
	}

	opts := defaultOpts(root)
	opts.Hidden = HiddenExclude
	res, err := Scan(opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Symlinks != 3 {
		t.Errorf("Symlinks = %d, want 3", res.Summary.Symlinks)
	}
	if res.Summary.Files != 2 { // real.txt + target/inner.txt; links not followed
		t.Errorf("Files = %d, want 2", res.Summary.Files)
	}
}

func TestScanGitignore(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	t.Setenv("HOME", t.TempDir()) // isolate from the user's global core.excludesFile
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, root, ".gitignore", "*.log\nbuild/\n")
	write(t, root, "keep.txt", "x\n")
	write(t, root, "junk.log", "x\n")
	write(t, root, "build/out.bin", "x\n")
	write(t, root, "sub/.gitignore", "local.txt\n")
	write(t, root, "sub/local.txt", "x\n")
	write(t, root, "sub/kept.txt", "x\n")

	// exclude mode drops ignored paths (nested .gitignore honored).
	opts := defaultOpts(root)
	opts.Ignored = IgnoredExclude
	opts.Hidden = HiddenExclude // keep .git and .gitignore files out of the stats
	res, err := Scan(opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Files != 2 {
		t.Errorf("exclude mode Files = %d, want 2", res.Summary.Files)
	}
	if res.Summary.Directories != 2 { // root + sub (build pruned)
		t.Errorf("exclude mode Directories = %d, want 2", res.Summary.Directories)
	}

	// only mode keeps only ignored paths; files under ignored dirs count.
	opts.Ignored = IgnoredOnly
	res, err = Scan(opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Files != 3 { // junk.log, build/out.bin, sub/local.txt
		t.Errorf("only mode Files = %d, want 3", res.Summary.Files)
	}

	// include mode does no matching at all.
	opts.Ignored = IgnoredInclude
	res, err = Scan(opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Files != 5 {
		t.Errorf("include mode Files = %d, want 5", res.Summary.Files)
	}
}

func TestScanNoWorktreeBehavesAsNoPatterns(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	t.Setenv("HOME", t.TempDir()) // isolate from the user's global core.excludesFile
	root := t.TempDir()
	write(t, root, ".gitignore", "*.log\n")
	write(t, root, "junk.log", "x\n")

	opts := defaultOpts(root)
	opts.Ignored = IgnoredExclude
	res, err := Scan(opts)
	if err != nil {
		t.Fatal(err)
	}
	// t.TempDir is not inside a git work tree, so no patterns apply.
	if res.Summary.Files != 2 {
		t.Errorf("Files = %d, want 2 (no work tree, no matching)", res.Summary.Files)
	}
}

func TestScanTypeFilterAndNoExt(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	write(t, root, "a.txt", "hello\n")
	write(t, root, "blob.bin", "\x00")
	write(t, root, "textnoext", "plain text\n")
	write(t, root, "binnoext", "\x00\x01")

	opts := defaultOpts(root)
	opts.Method = "hybrid"
	opts.TypeFilter = TypeText
	opts.ListNoExt = true
	res, err := Scan(opts)
	if err != nil {
		t.Fatal(err)
	}
	// Files counts everything readable that passed traversal filters.
	if res.Summary.Files != 4 {
		t.Errorf("Files = %d, want 4", res.Summary.Files)
	}
	// FilesWithoutExtension counts only files that passed --type text.
	if res.Summary.FilesWithoutExtension != 1 {
		t.Errorf("FilesWithoutExtension = %d, want 1", res.Summary.FilesWithoutExtension)
	}
	if !reflect.DeepEqual(res.NoExtFiles, []string{"textnoext"}) {
		t.Errorf("NoExtFiles = %v, want [textnoext]", res.NoExtFiles)
	}
	// Both extensionless files, plus blob.bin whose extension misses the
	// text-extensions list and is therefore sniffed too (R13).
	if res.Summary.FilesSniffed != 3 {
		t.Errorf("FilesSniffed = %d, want 3", res.Summary.FilesSniffed)
	}
	for _, g := range res.Groups {
		if !g.Text {
			t.Errorf("binary group %q survived --type text", g.Format)
		}
	}
}

func TestScanNoExtFilesSorted(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	// DFS order would yield b/c before b-y; the list must be sorted.
	write(t, root, "b/c", "x\n")
	write(t, root, "b-y", "x\n")
	write(t, root, "a", "x\n")

	opts := defaultOpts(root)
	opts.ListNoExt = true
	res, err := Scan(opts)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b-y", "b/c"}
	if !reflect.DeepEqual(res.NoExtFiles, want) {
		t.Errorf("NoExtFiles = %v, want %v", res.NoExtFiles, want)
	}
}

func TestScanRootUnreadable(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	missing := filepath.Join(root, "nope")
	if _, err := Scan(defaultOpts(missing)); err == nil {
		t.Fatal("expected error scanning missing root")
	}
}

func TestCountLOC(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	dir := t.TempDir()
	tests := []struct {
		name    string
		content string
		want    int64
	}{
		{"empty", "", 0},
		{"one line with newline", "hello\n", 1},
		{"one line no newline", "hello", 1},
		{"three lines", "a\nb\nc\n", 3},
		{"trailing partial", "a\nb\nc", 3},
		{"only newlines", "\n\n\n", 3},
	}
	buf := make([]byte, 8) // tiny buffer to exercise chunk boundaries
	for _, tc := range tests {
		p := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "_"))
		if err := os.WriteFile(p, []byte(tc.content), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := countLOC(p, buf)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: countLOC = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestScanDeterministicAcrossWorkerCounts(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	for i := range 20 {
		write(t, root, filepath.Join("d", string(rune('a'+i))+".go"), strings.Repeat("line\n", i+1))
	}
	opts1 := defaultOpts(root)
	opts1.Workers = 1
	opts8 := defaultOpts(root)
	opts8.Workers = 8

	r1, err := Scan(opts1)
	if err != nil {
		t.Fatal(err)
	}
	r8, err := Scan(opts8)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r1, r8) {
		t.Errorf("results differ across worker counts:\n1: %+v\n8: %+v", r1, r8)
	}
}
