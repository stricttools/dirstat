package test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stricttools/dirstat/internal/testutil"
	"github.com/stricttools/testisolation/go/hygiene"
)

// --formats canonical merges alias formats into one group and resolves
// extensionless scripts by their shebang interpreter; --formats raw (the
// default) keeps every group name exactly as the extension or the sniffed
// MIME type produced it.

func TestCanonicalMergesAliasFormats(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"a.mjs":   "export const a = 1;\n",
		"b.js":    "const b = 2;\n",
		"lib.h":   "#pragma once\n",
		"lib.c":   "int main(void) { return 0; }\n",
		"cls.hpp": "#pragma once\n",
	})

	raw := scanJSON(t, "scan", root, "--json", "--formats", "raw",
		"--sort-by", "format", "--sort-order", "asc")
	if got := strings.Join(groupFormats(t, raw), ","); got != "c,h,hpp,js,mjs" {
		t.Errorf("raw formats = %s, want c,h,hpp,js,mjs", got)
	}

	canon := scanJSON(t, "scan", root, "--json", "--formats", "canonical",
		"--sort-by", "format", "--sort-order", "asc")
	if got := strings.Join(groupFormats(t, canon), ","); got != "c,cpp,js" {
		t.Errorf("canonical formats = %s, want c,cpp,js", got)
	}
	if g := findGroup(t, canon, "js"); g["count"] != float64(2) {
		t.Errorf("canonical js count = %v, want 2 (a.mjs + b.js)", g["count"])
	}
	if g := findGroup(t, canon, "c"); g["count"] != float64(2) {
		t.Errorf("canonical c count = %v, want 2 (lib.h + lib.c)", g["count"])
	}
	if got := summaryField(t, canon, "unique_formats"); got != 3 {
		t.Errorf("canonical unique_formats = %d, want 3", got)
	}
}

func TestCanonicalResolvesShebangInterpreter(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		// The uv runner form sniffs as plain text, so only the shebang can
		// name this script's format.
		"tool": "#!/usr/bin/env -S uv run python\nprint('hi')\n",
	})

	raw := scanJSON(t, "scan", root, "--json", "--formats", "raw")
	if got := strings.Join(groupFormats(t, raw), ","); got != "text/plain" {
		t.Errorf("raw formats = %s, want text/plain", got)
	}

	canon := scanJSON(t, "scan", root, "--json", "--formats", "canonical")
	if got := strings.Join(groupFormats(t, canon), ","); got != "py" {
		t.Errorf("canonical formats = %s, want py", got)
	}
	if g := findGroup(t, canon, "py"); g["text"] != true {
		t.Errorf("canonical py group: text = %v, want true", g["text"])
	}
}

func TestFormatsDefaultsToRaw(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"a.mjs":  "export const a = 1;\n",
		"b.js":   "const b = 2;\n",
		"script": "#!/usr/bin/env python3\nprint('hi')\n",
	})
	omitted := groupFormats(t, scanJSON(t, "scan", root, "--json",
		"--sort-by", "format", "--sort-order", "asc"))
	explicit := groupFormats(t, scanJSON(t, "scan", root, "--json", "--formats", "raw",
		"--sort-by", "format", "--sort-order", "asc"))
	if strings.Join(omitted, ",") != strings.Join(explicit, ",") {
		t.Errorf("omitted --formats = %v, explicit raw = %v", omitted, explicit)
	}
	if strings.Join(omitted, ",") != "js,mjs,text/x-python" {
		t.Errorf("raw formats = %v, want [js mjs text/x-python]", omitted)
	}
}

func TestFormatsFromConfigFile(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"a.mjs":       "export const a = 1;\n",
		"b.js":        "const b = 2;\n",
		"dirstat.tml": "formats = \"canonical\"\n",
	})
	cfg := filepath.Join(root, "dirstat.tml")

	parsed := scanJSON(t, "scan", root, "--json", "--config", cfg,
		"--sort-by", "format", "--sort-order", "asc")
	if got := strings.Join(groupFormats(t, parsed), ","); got != "js,tml" {
		t.Errorf("formats = %s, want js,tml (the config file's canonical mode)", got)
	}

	// The same key on both sides is the documented conflict, not a silent
	// override (R43).
	_, stderr, code := runDirstat(t, "scan", root, "--config", cfg, "--formats", "raw")
	if code != 2 || !strings.Contains(stderr, "--formats") {
		t.Errorf("config/flag conflict: exit %d, stderr %q", code, stderr)
	}
}
