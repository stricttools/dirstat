package test

import (
	"github.com/stricttools/testisolation/go/hygiene"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/smm-h/dirstat/internal/testutil"
)

// writeConfigFile writes a TOML scan config into dir and returns its path.
func writeConfigFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "dirstat.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigHappyPath(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"a.go":        "package a\n",
		"skipme/b.go": "package b\n",
		"deep/c/d.go": "package d\n",
		"Makefile":    "all:\n",
	})
	cfgPath := writeConfigFile(t, t.TempDir(), "exclude = [\"skipme\"]\nmethod = \"ext\"\n")

	// File sets exclude+method; CLI sets --depth for a key the file does not
	// set: both take effect together (R43, R44).
	parsed := scanJSON(t, "scan", root, "--json", "--config", cfgPath, "--depth", "1")
	if got := parsed["method"]; got != "ext" {
		t.Errorf("method = %v, want ext (from config file)", got)
	}
	if got := summaryField(t, parsed, "files"); got != 2 { // a.go, Makefile
		t.Errorf("files = %d, want 2 (skipme excluded by file, deep pruned by --depth)", got)
	}
	// Method ext: the extensionless Makefile is never sniffed.
	if got := summaryField(t, parsed, "files_sniffed"); got != 0 {
		t.Errorf("files_sniffed = %d, want 0 with method ext from config", got)
	}
	abs, _ := filepath.Abs(cfgPath)
	if got := parsed["config"]; got != abs {
		t.Errorf("config = %v, want absolute path %q", got, abs)
	}
}

func TestConfigAbsentFromJSONWithoutFlag(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{"a.go": "package a\n"})
	parsed := scanJSON(t, "scan", root, "--json")
	if _, present := parsed["config"]; present {
		t.Errorf("config field must be absent without --config: %v", parsed)
	}
}

func TestConfigSummaryLine(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{"a.go": "package a\n"})
	cfgPath := writeConfigFile(t, t.TempDir(), "method = \"ext\"\n")
	abs, _ := filepath.Abs(cfgPath)

	stdout, stderr, code := runDirstat(t, "scan", root, "--config", cfgPath)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	cfgIdx := strings.Index(stdout, "Config: "+abs)
	if cfgIdx < 0 {
		t.Fatalf("missing 'Config: %s' summary line:\n%s", abs, stdout)
	}
	if dirIdx := strings.Index(stdout, "Directories:"); cfgIdx > dirIdx {
		t.Errorf("Config must be the first summary line:\n%s", stdout)
	}

	stdout, _, _ = runDirstat(t, "scan", root)
	if strings.Contains(stdout, "Config:") {
		t.Errorf("Config line must be absent without --config:\n%s", stdout)
	}
}

func TestConfigFileErrors(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{"a.go": "package a\n"})

	tests := []struct {
		name    string
		content string
		wantErr []string
	}{
		{"malformed TOML", "exclude = [\nmethod =", []string{"line", "column"}},
		{"unknown key", "bogus = 1\n", []string{`"bogus"`, "sort_order"}},
		{"rendering key", "colors = true\n", []string{`"colors"`, "rendering", "sort_order"}},
		{"where key", "where = \"/x\"\n", []string{`"where"`, "positional"}},
		{"wrong type", "exclude = \"str\"\n", []string{`"exclude"`, "array"}},
		{"invalid choice", "method = \"x\"\n", []string{`"method"`, `"x"`}},
		{"invalid stat", "stats = [\"bogus\"]\n", []string{`"stats"`, `"bogus"`}},
		{"invalid sort column", "sort_by = [\"sizes\"]\n", []string{`"sort_by"`, `"sizes"`}},
		{"duplicate value", "exclude = [\"a\", \"a\"]\n", []string{`"exclude"`, "duplicate"}},
	}
	for _, tc := range tests {
		cfgPath := writeConfigFile(t, t.TempDir(), tc.content)
		_, stderr, code := runDirstat(t, "scan", root, "--config", cfgPath)
		if code != 2 {
			t.Errorf("%s: exit = %d, want 2 (stderr: %s)", tc.name, code, stderr)
		}
		for _, want := range tc.wantErr {
			if !strings.Contains(stderr, want) {
				t.Errorf("%s: stderr %q missing %q", tc.name, stderr, want)
			}
		}
		if !strings.Contains(stderr, cfgPath) {
			t.Errorf("%s: stderr %q missing the config path", tc.name, stderr)
		}
	}

	// Truncated mid-array at EOF: the parse error must carry a real,
	// non-zero line/column position (R41). go-toml-edit < 0.2.2 reported
	// "line 0, column 0" for unexpected-EOF errors.
	cfgPath := writeConfigFile(t, t.TempDir(), "exclude = [\"a\", \"b\"")
	_, stderr, code := runDirstat(t, "scan", root, "--config", cfgPath)
	if code != 2 {
		t.Errorf("truncated at EOF: exit = %d, want 2 (stderr: %s)", code, stderr)
	}
	if !regexp.MustCompile(`line [1-9]`).MatchString(stderr) {
		t.Errorf("truncated at EOF: stderr %q must report a non-zero line position", stderr)
	}

	// Missing file: hard error reporting the path (R41).
	missing := filepath.Join(t.TempDir(), "nope.toml")
	_, stderr, code = runDirstat(t, "scan", root, "--config", missing)
	if code != 2 || !strings.Contains(stderr, missing) {
		t.Errorf("missing file: exit %d, stderr %q", code, stderr)
	}
}

func TestConfigCLIConflict(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{"a.go": "package a\n"})

	tests := []struct {
		name    string
		content string
		args    []string
		wantKey string
	}{
		{"exclude space form", "exclude = [\"x\"]\n", []string{"--exclude", "y"}, "exclude"},
		{"exclude equals form", "exclude = [\"x\"]\n", []string{"--exclude=y"}, "exclude"},
		{"method", "method = \"ext\"\n", []string{"--method", "hybrid"}, "method"},
		{"sort_by", "sort_by = [\"format\"]\n", []string{"--sort-by=count"}, "sort_by"},
	}
	for _, tc := range tests {
		cfgPath := writeConfigFile(t, t.TempDir(), tc.content)
		args := append([]string{"scan", root, "--config", cfgPath}, tc.args...)
		_, stderr, code := runDirstat(t, args...)
		if code != 2 {
			t.Errorf("%s: exit = %d, want 2 (stderr: %s)", tc.name, code, stderr)
		}
		if !strings.Contains(stderr, `"`+tc.wantKey+`"`) {
			t.Errorf("%s: stderr %q does not name the key %q", tc.name, stderr, tc.wantKey)
		}
	}

	// A CLI flag for a key the file does not set stays usable (R43).
	cfgPath := writeConfigFile(t, t.TempDir(), "method = \"ext\"\n")
	_, stderr, code := runDirstat(t, "scan", root, "--config", cfgPath, "--exclude", "x")
	if code != 0 {
		t.Errorf("non-overlapping CLI flag alongside --config must work: exit %d, stderr %s", code, stderr)
	}
}

func TestConfigEmptyExcludeScansEverything(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		".git/config":       "[core]\n",
		"node_modules/x.js": "x\n",
		"a.go":              "package a\n",
	})
	// exclude = [] replaces the built-in default list entirely: .git and
	// node_modules become visible again (R45).
	cfgPath := writeConfigFile(t, t.TempDir(), "exclude = []\n")
	parsed := scanJSON(t, "scan", root, "--json", "--config", cfgPath, "--ignored", "include")
	if got := summaryField(t, parsed, "files"); got != 3 {
		t.Errorf("files = %d, want 3 (exclude = [] must scan everything)", got)
	}
}
