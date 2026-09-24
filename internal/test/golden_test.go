package test

import (
	"encoding/json"
	"flag"
	"github.com/stricttools/testisolation/go/hygiene"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/smm-h/dirstat/internal/testutil"
)

var update = flag.Bool("update", false, "rewrite golden files from current output")

// goldenTree builds the fixed fixture tree used by the golden tests. It
// deliberately avoids symlinks and permission tricks so the JSON is
// identical on every platform.
func goldenTree(t *testing.T) string {
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"a.go":       "package main\n\nfunc main() {}\n",
		"b.go":       "package b\n",
		"README.md":  "# Title\n\nBody text.\n",
		"data.bin":   "\x00\x01\x02\x03\x04",
		"Makefile":   "all:\n\techo hi\n",
		"sub/c.txt":  "one\ntwo\n",
		"sub/empty/": "",
	})
	return root
}

// normalize turns a machine-mode run's stdout into the stable text the golden
// files hold: the envelope, re-indented (the framework writes it on one line),
// with the run-specific binary version and absolute root replaced by
// placeholders. Re-encoding sorts the keys, which is deliberate -- the
// envelope's field order is cosmetic, so a golden file should not assert it.
func normalize(t *testing.T, out, root string) string {
	t.Helper()
	var env map[string]interface{}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\n%s", err, out)
	}
	if _, ok := env["app_version"]; ok {
		env["app_version"] = "VERSION"
	}
	pretty, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	// The root only appears in the "root" field; paths in the document are
	// relative.
	rootJSON, _ := filepath.Abs(root)
	return regexp.MustCompile(regexp.QuoteMeta(rootJSON)).ReplaceAllString(string(pretty)+"\n", "ROOT")
}

func checkGolden(t *testing.T, goldenName string, args ...string) {
	t.Helper()
	root := goldenTree(t)
	stdout, stderr, code := runDirstat(t, append([]string{"scan", root}, args...)...)
	if code != 0 {
		t.Fatalf("dirstat exited %d: %s", code, stderr)
	}
	compareGolden(t, goldenName, normalize(t, stdout, root))
}

// compareGolden compares normalized output against a committed golden file,
// rewriting it under -update.
func compareGolden(t *testing.T, goldenName, got string) {
	t.Helper()
	goldenPath := filepath.Join("testdata", goldenName)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading golden file (run with -update to create): %v", err)
	}
	if got != string(want) {
		t.Errorf("output differs from %s:\n--- got ---\n%s\n--- want ---\n%s", goldenPath, got, want)
	}
}

func TestGoldenJSONFull(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	checkGolden(t, "full.json", "--json", "--list-no-ext")
}

func TestGoldenJSONConfig(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	// Exercises the additive "config" JSON field (R46). The config file
	// lives inside the fixture root so its path normalizes to ROOT/... and
	// its own toml group is a deterministic part of the golden output.
	root := goldenTree(t)
	cfgPath := filepath.Join(root, "dirstat.toml")
	content := "method = \"ext\"\nsort_by = [\"format\"]\nsort_order = \"asc\"\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runDirstat(t, "scan", root, "--config", cfgPath, "--json")
	if code != 0 {
		t.Fatalf("dirstat exited %d: %s", code, stderr)
	}
	compareGolden(t, "config.json", normalize(t, stdout, root))
}

func TestGoldenJSONSubset(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	checkGolden(t, "subset.json",
		"--json",
		"--stats", "count", "--stats", "total-size", "--stats", "total-loc",
		"--type", "text",
		"--sort-by", "format", "--sort-order", "asc",
		"--method", "ext")
}
