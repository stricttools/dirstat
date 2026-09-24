package classify

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stricttools/testisolation/go/hygiene"
)

func TestShebangFormat(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	tests := []struct {
		line string
		want string // "" means no mapping
	}{
		// Plain interpreter paths.
		{"#!/bin/sh", "sh"},
		{"#!/bin/bash", "sh"},
		{"#!/usr/bin/zsh", "sh"},
		{"#!/bin/dash", "sh"},
		{"#!/bin/ksh", "sh"},
		{"#!/usr/bin/fish", "fish"},
		{"#!/usr/bin/perl", "pl"},
		{"#!/usr/bin/ruby", "rb"},
		{"#!/usr/bin/php", "php"},
		{"#!/usr/bin/lua", "lua"},
		{"#!/usr/bin/awk -f", "awk"},
		{"#!/usr/bin/gawk -f", "awk"},
		{"#!/usr/bin/Rscript", "r"},
		{"#!/usr/local/bin/node", "js"},
		{"#!/usr/bin/deno run", "js"},
		{"#!/usr/bin/bun", "js"},
		// Version suffixes are stripped.
		{"#!/usr/bin/python3", "py"},
		{"#!/usr/bin/python3.12", "py"},
		{"#!/usr/bin/python2.7", "py"},
		{"#!/usr/bin/node20", "js"},
		// env wrappers, including flags and NAME=value assignments.
		{"#!/usr/bin/env python", "py"},
		{"#!/usr/bin/env  bash", "sh"},
		{"#!/usr/bin/env -i node", "js"},
		{"#!/usr/bin/env -u LD_PRELOAD ruby", "rb"},
		{"#!/usr/bin/env FOO=bar perl", "pl"},
		{"#!/usr/bin/env -S python3 -X utf8", "py"},
		{"#!/usr/bin/env --split-string python3 -u", "py"},
		{"#!/usr/bin/env -Spython3", "py"},
		{"#!/usr/bin/env --split-string=python3 -u", "py"},
		// Runner forms.
		{"#!/usr/bin/env -S uv run python", "py"},
		{"#!/usr/bin/env uv run python3.12", "py"},
		{"#!/usr/bin/env -S uvx ruby", "rb"},
		{"#!/usr/bin/uv run node", "js"},
		// Unknown interpreters and non-shebang lines map to nothing.
		{"#!/usr/bin/env pwsh", ""},
		{"#!/usr/bin/tclsh", ""},
		{"#!/usr/bin/env", ""},
		{"#!/usr/bin/env -S uv run --with rich python", ""},
		{"#!", ""},
		{"#!   ", ""},
		{"", ""},
		{"print('hi')", ""},
		{" #!/bin/sh", ""},
	}
	for _, tc := range tests {
		got, ok := ShebangFormat(tc.line)
		if tc.want == "" {
			if ok {
				t.Errorf("ShebangFormat(%q) = %q, want no mapping", tc.line, got)
			}
			continue
		}
		if !ok || got != tc.want {
			t.Errorf("ShebangFormat(%q) = %q (ok=%v), want %q", tc.line, got, ok, tc.want)
		}
	}
}

func TestReadShebang(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	dir := t.TempDir()
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"script", "#!/bin/sh\necho hi\n", "#!/bin/sh"},
		{"crlf", "#!/bin/sh\r\necho hi\r\n", "#!/bin/sh"},
		{"oneline", "#!/bin/sh", "#!/bin/sh"},
		{"plain", "hello\n", ""},
		{"empty", "", ""},
		{"binary", "\x00\x01\x02", ""},
	}
	for _, tc := range tests {
		p := writeFile(t, dir, tc.name, []byte(tc.content))
		if got := readShebang(p); got != tc.want {
			t.Errorf("readShebang(%s) = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := readShebang(filepath.Join(dir, "missing")); got != "" {
		t.Errorf("readShebang on a missing file = %q, want \"\"", got)
	}
	// Only the first bytes are read: a file whose first line runs past the
	// read limit yields the part that was read, and nothing beyond it.
	long := writeFile(t, dir, "long", []byte("#!/usr/bin/env "+strings.Repeat("x", 2*shebangReadLimit)+"\npython\n"))
	if got := readShebang(long); len(got) != shebangReadLimit {
		t.Errorf("readShebang(long) returned %d bytes, want %d", len(got), shebangReadLimit)
	}
}

func TestCanonicalGrouping(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	dir := t.TempDir()
	mjs := writeFile(t, dir, "mod.mjs", []byte("export const a = 1;\n"))
	header := writeFile(t, dir, "lib.h", []byte("#pragma once\n"))
	pyScript := writeFile(t, dir, "tool", []byte("#!/usr/bin/env -S uv run python\nprint('hi')\n"))
	shScript := writeFile(t, dir, "runme", []byte("#!/bin/sh\necho hi\n"))
	unknown := writeFile(t, dir, "weirdo", []byte("#!/usr/bin/env pwsh\nWrite-Host hi\n"))
	csh := writeFile(t, dir, "cshscript", []byte("#!/bin/csh\necho hi\n"))

	tests := []struct {
		method    string
		formats   string
		path      string
		wantGroup string
		wantText  bool
	}{
		// raw: every group name is exactly what the extension or the sniff said
		{MethodHybrid, FormatsRaw, mjs, "mjs", true},
		{MethodHybrid, FormatsRaw, header, "h", true},
		{MethodHybrid, FormatsRaw, pyScript, "text/plain", true},
		{MethodHybrid, FormatsRaw, shScript, "text/x-shellscript", true},
		// canonical: alias table on extensions, shebang on extensionless files
		{MethodHybrid, FormatsCanonical, mjs, "js", true},
		{MethodHybrid, FormatsCanonical, header, "c", true},
		{MethodHybrid, FormatsCanonical, pyScript, "py", true},
		{MethodHybrid, FormatsCanonical, shScript, "sh", true},
		// an unknown interpreter falls through to the sniff: pwsh has no
		// signature and answers text/plain, csh sniffs as a shell script and
		// that MIME type still goes through the alias table
		{MethodHybrid, FormatsCanonical, unknown, "text/plain", true},
		{MethodHybrid, FormatsCanonical, csh, "sh", true},
		{MethodHybrid, FormatsRaw, csh, "text/x-shellscript", true},
		// type sniffs everything; the alias table and the shebang apply there too
		{MethodType, FormatsCanonical, shScript, "sh", true},
		{MethodType, FormatsCanonical, pyScript, "py", true},
		{MethodType, FormatsRaw, shScript, "text/x-shellscript", true},
		// ext never reads content, so an extensionless file stays unnamed
		{MethodExt, FormatsCanonical, pyScript, "(no extension)", false},
		{MethodExt, FormatsCanonical, mjs, "js", false},
		{MethodExt, FormatsRaw, mjs, "mjs", false},
	}
	for _, tc := range tests {
		c := testCanonicalClassifier(tc.method, tc.formats)
		cls, err := c.File(tc.path, filepath.Base(tc.path), fileSize(t, tc.path))
		if err != nil {
			t.Errorf("%s/%s %s: unexpected error: %v", tc.method, tc.formats, tc.path, err)
			continue
		}
		if cls.Group != tc.wantGroup || cls.Text != tc.wantText {
			t.Errorf("%s/%s %s: got group=%q text=%v, want group=%q text=%v",
				tc.method, tc.formats, filepath.Base(tc.path), cls.Group, cls.Text, tc.wantGroup, tc.wantText)
		}
	}
}
