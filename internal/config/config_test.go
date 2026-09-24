package config

import (
	"testing"

	"github.com/stricttools/testisolation/go/hygiene"
)

func TestTextExtensions(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	exts := TextExtensions()
	if len(exts) < 90 {
		t.Fatalf("expected at least 90 text extensions, got %d", len(exts))
	}
	for _, tc := range []string{"svg", "go", "py", "md", "json", "sh", "makefile", "gitignore"} {
		if _, ok := exts[tc]; !ok {
			t.Errorf("expected extension %q in text extensions list", tc)
		}
	}
	for _, tc := range []string{"png", "exe", "", "#"} {
		if _, ok := exts[tc]; ok {
			t.Errorf("did not expect %q in text extensions list", tc)
		}
	}
	// Formats reported missing from the list: Godot's text formats, Go's
	// module files, and a batch of web/shader/schema languages.
	for _, tc := range []string{
		"gd", "tscn", "tres", "gdshader", "godot", "uid",
		"mod", "sum",
		"svx", "astro", "prisma", "odin",
		"wgsl", "glsl", "hlsl", "frag", "vert",
	} {
		if _, ok := exts[tc]; !ok {
			t.Errorf("expected extension %q in text extensions list", tc)
		}
	}
	// `lock` is deliberately absent: uv and Poetry lockfiles are TOML text
	// while other tools write binary .lock files, so hybrid mode must sniff
	// each one instead of trusting a static entry (R13).
	if _, ok := exts["lock"]; ok {
		t.Error("lock must stay out of the text extensions list so it is sniffed per file")
	}
}

func TestTextMimetypes(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	mimes := TextMimetypes()
	if len(mimes) < 40 {
		t.Fatalf("expected at least 40 text mimetypes, got %d", len(mimes))
	}
	for _, tc := range []string{"image/svg+xml", "application/json", "application/x-empty", "inode/x-empty"} {
		if _, ok := mimes[tc]; !ok {
			t.Errorf("expected mimetype %q in text mimetypes list", tc)
		}
	}
	if _, ok := mimes["image/png"]; ok {
		t.Error("did not expect image/png in text mimetypes list")
	}
}

func TestLoadTheme(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	tests := []struct {
		name   string
		textFG string
	}{
		{"dark", "114"},
		{"light", "28"},
	}
	for _, tc := range tests {
		th := LoadTheme(tc.name)
		if th.TextFG != tc.textFG {
			t.Errorf("theme %s: TextFG = %q, want %q", tc.name, th.TextFG, tc.textFG)
		}
		if th.Border == "" || th.HeaderFG == "" || th.StatLabel == "" || th.StatValue == "" || th.Error == "" {
			t.Errorf("theme %s: unexpected empty required color: %+v", tc.name, th)
		}
	}
}

func TestLoadThemeUnknownPanics(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for unknown theme name")
		}
	}()
	LoadTheme("solarized")
}

func TestCanonicalFormats(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	aliases := CanonicalFormats()
	for from, want := range map[string]string{
		"mjs": "js", "cjs": "js", "mts": "ts", "cts": "ts",
		"h": "c", "hh": "cpp", "hpp": "cpp",
		"text/x-python": "py", "text/x-shellscript": "sh",
		"text/x-perl": "pl", "text/x-ruby": "rb", "text/x-php": "php",
		"text/x-lua": "lua", "text/x-tcl": "tcl",
	} {
		if got := aliases[from]; got != want {
			t.Errorf("CanonicalFormats()[%q] = %q, want %q", from, got, want)
		}
	}
	// Names that must stay untouched: an alias target may not itself be an
	// alias, or a group would merge twice depending on lookup order.
	for _, from := range []string{"js", "ts", "c", "cpp", "py", "sh", "text/plain", ""} {
		if to, ok := aliases[from]; ok {
			t.Errorf("CanonicalFormats() maps %q to %q; alias targets must be terminal", from, to)
		}
	}
}

func TestParsePairs(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	pairs, err := parsePairs("# comment\n\n  mjs js \n.CJS .JS\ntext/x-python py\n")
	if err != nil {
		t.Fatalf("parsePairs: %v", err)
	}
	want := map[string]string{"mjs": "js", "cjs": "js", "text/x-python": "py"}
	for from, w := range want {
		if pairs[from] != w {
			t.Errorf("parsePairs()[%q] = %q, want %q", from, pairs[from], w)
		}
	}
	if len(pairs) != len(want) {
		t.Errorf("parsePairs() = %v, want exactly %v", pairs, want)
	}

	// A malformed line is a hard error, never a skipped line.
	for _, bad := range []string{"mjs\n", "mjs js cjs\n", "ok fine\nmjs\n", ". js\n", "mjs js\nmjs ts\n"} {
		if _, err := parsePairs(bad); err == nil {
			t.Errorf("parsePairs(%q) succeeded, want an error", bad)
		}
	}
}
