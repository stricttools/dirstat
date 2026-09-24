package classify

import (
	"github.com/stricttools/testisolation/go/hygiene"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeExt(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	tests := []struct {
		name string
		want string
	}{
		{"main.go", "go"},
		{"archive.tar.gz", "gz"},
		{"UPPER.TXT", "txt"},
		{".bashrc", ""},
		{".gitignore", ""},
		{"Makefile", ""},
		{"file.", ""},
		{"..", ""},
		{".hidden.txt", "txt"},
		{"noext", ""},
		{"a.B", "b"},
	}
	for _, tc := range tests {
		if got := NormalizeExt(tc.name); got != tc.want {
			t.Errorf("NormalizeExt(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func testClassifier(method string) *Classifier {
	return testCanonicalClassifier(method, FormatsRaw)
}

// testCanonicalClassifier builds a classifier with a small alias table, so the
// canonical-mode tests never depend on the real embedded one.
func testCanonicalClassifier(method, formats string) *Classifier {
	exts := map[string]struct{}{"go": {}, "txt": {}, "md": {}, "svg": {}}
	mimes := map[string]struct{}{"application/json": {}, "image/svg+xml": {}}
	aliases := map[string]string{
		"mjs": "js", "h": "c", "text/x-shellscript": "sh",
	}
	return New(method, formats, exts, mimes, aliases)
}

func TestMimeIsText(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	c := testClassifier(MethodType)
	tests := []struct {
		mime string
		want bool
	}{
		{"text/plain", true},
		{"text/x-shellscript", true},
		{"application/json", true},
		{"image/svg+xml", true},
		{"application/octet-stream", false},
		{"image/png", false},
	}
	for _, tc := range tests {
		if got := c.MimeIsText(tc.mime); got != tc.want {
			t.Errorf("MimeIsText(%q) = %v, want %v", tc.mime, got, tc.want)
		}
	}
}

// fileSize returns the on-disk size of path, which the classifier needs to
// apply the empty-file rule.
func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Size()
}

func writeFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFileByMethod(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	dir := t.TempDir()
	goFile := writeFile(t, dir, "main.go", []byte("package main\n"))
	binFile := writeFile(t, dir, "blob.bin", []byte{0, 1, 2, 3})
	script := writeFile(t, dir, "runme", []byte("#!/bin/sh\necho hi\n"))
	binNoExt := writeFile(t, dir, "rawdata", []byte{0, 1, 2, 3})

	tests := []struct {
		method    string
		path      string
		wantGroup string
		wantText  bool
		wantNoExt bool
		wantSniff bool
	}{
		// ext: extension list only, never sniffs
		{MethodExt, goFile, "go", true, false, false},
		{MethodExt, binFile, "bin", false, false, false},
		{MethodExt, script, "(no extension)", false, true, false},
		// type: everything sniffed
		{MethodType, goFile, "text/plain", true, false, true},
		{MethodType, binNoExt, "application/octet-stream", false, true, true},
		{MethodType, script, "text/x-shellscript", true, true, true},
		// hybrid: a text-extension hit wins outright; a map miss (bin) is
		// sniffed, keeping the extension as the group name (R13)
		{MethodHybrid, goFile, "go", true, false, false},
		{MethodHybrid, binFile, "bin", false, false, true},
		{MethodHybrid, script, "text/x-shellscript", true, true, true},
		{MethodHybrid, binNoExt, "application/octet-stream", false, true, true},
	}
	for _, tc := range tests {
		c := testClassifier(tc.method)
		cls, err := c.File(tc.path, filepath.Base(tc.path), fileSize(t, tc.path))
		if err != nil {
			t.Errorf("%s %s: unexpected error: %v", tc.method, tc.path, err)
			continue
		}
		if cls.Group != tc.wantGroup || cls.Text != tc.wantText || cls.NoExt != tc.wantNoExt || cls.Sniffed != tc.wantSniff {
			t.Errorf("%s %s: got %+v, want group=%q text=%v noext=%v sniffed=%v",
				tc.method, filepath.Base(tc.path), cls, tc.wantGroup, tc.wantText, tc.wantNoExt, tc.wantSniff)
		}
	}
}

func TestHybridSniffsOnExtensionMapMiss(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	// An extension that is absent from the text-extensions list must not be
	// assumed binary: hybrid sniffs the content and lets the MIME rule
	// decide, while the group name stays the extension (R13).
	dir := t.TempDir()
	gd := writeFile(t, dir, "player.gd", []byte("extends Node\n\nfunc _ready():\n\tprint(\"hi\")\n"))
	lock := writeFile(t, dir, "uv.lock", []byte("version = 1\nrequires-python = \">=3.11\"\n"))
	blob := writeFile(t, dir, "asset.weird", []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0x00})

	tests := []struct {
		method    string
		path      string
		wantGroup string
		wantText  bool
		wantSniff bool
	}{
		// hybrid: map miss falls through to a content sniff
		{MethodHybrid, gd, "gd", true, true},
		{MethodHybrid, lock, "lock", true, true},
		{MethodHybrid, blob, "weird", false, true},
		// ext: never sniffs, so a map miss stays binary
		{MethodExt, gd, "gd", false, false},
		{MethodExt, lock, "lock", false, false},
	}
	for _, tc := range tests {
		c := testClassifier(tc.method)
		cls, err := c.File(tc.path, filepath.Base(tc.path), fileSize(t, tc.path))
		if err != nil {
			t.Errorf("%s %s: unexpected error: %v", tc.method, tc.path, err)
			continue
		}
		if cls.Group != tc.wantGroup || cls.Text != tc.wantText || cls.Sniffed != tc.wantSniff {
			t.Errorf("%s %s: got %+v, want group=%q text=%v sniffed=%v",
				tc.method, filepath.Base(tc.path), cls, tc.wantGroup, tc.wantText, tc.wantSniff)
		}
	}
}

func TestFileSniffErrorIsReported(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	dir := t.TempDir()
	missing := filepath.Join(dir, "gone")
	c := testClassifier(MethodType)
	cls, err := c.File(missing, "gone", 1)
	if err == nil {
		t.Fatal("expected error sniffing missing file")
	}
	if !cls.Sniffed {
		t.Error("expected Sniffed=true even on sniff error")
	}
}

func TestEmptyFileIsText(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	// A zero-byte file has no bytes that could make it binary, so it is text
	// under every method (R14). Where the extension decides, the size
	// short-circuits the verdict and no content is read at all; where the
	// method groups by MIME type the sniff still runs to name the group, but
	// its verdict cannot make an empty file binary.
	dir := t.TempDir()
	tests := []struct {
		method    string
		name      string
		wantGroup string
		wantSniff bool
	}{
		{MethodHybrid, "empty", "text/plain", true},
		{MethodHybrid, "empty.png", "png", false},
		{MethodHybrid, "empty.go", "go", false},
		{MethodExt, "empty.png", "png", false},
		{MethodType, "empty.png", "text/plain", true},
	}
	for _, tc := range tests {
		empty := writeFile(t, dir, tc.name, nil)
		c := testClassifier(tc.method)
		cls, err := c.File(empty, tc.name, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !cls.Text || cls.Sniffed != tc.wantSniff || cls.Group != tc.wantGroup {
			t.Errorf("%s %s: got %+v, want group=%q text=true sniffed=%v",
				tc.method, tc.name, cls, tc.wantGroup, tc.wantSniff)
		}
	}
}
