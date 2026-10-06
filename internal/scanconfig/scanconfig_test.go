package scanconfig

import (
	"github.com/stricttools/testisolation/go/hygiene"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// writeConfig writes a TOML config file into a temp dir and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dirstat.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// loadErr loads a config expected to fail and returns the error message.
func loadErr(t *testing.T, content string) string {
	t.Helper()
	path := writeConfig(t, content)
	_, err := Load(path)
	if err == nil {
		t.Fatalf("Load succeeded, want error, for config:\n%s", content)
	}
	return err.Error()
}

func TestLoadHappyAllKeys(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	path := writeConfig(t, `
exclude = [".git", "node_modules"]
method = "ext"
formats = "canonical"
depth = 3
ignored = "include"
hidden = "exclude"
type = "text"
stats = ["count", "total-size"]
sort_by = ["format", "count"]
sort_order = "asc"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, key := range AllowedKeys {
		if !cfg.Has(key) {
			t.Errorf("Has(%q) = false, want true", key)
		}
	}
	kwargs := map[string]interface{}{
		"exclude": []interface{}{"old"}, "method": "hybrid", "formats": "raw", "depth": -1,
		"ignored": "exclude", "hidden": "include", "type": "both",
		"stats": []interface{}{}, "sort_by": []interface{}{"count"}, "sort_order": "desc",
	}
	cfg.Overlay(kwargs)
	want := map[string]interface{}{
		"exclude": []interface{}{".git", "node_modules"}, "method": "ext", "formats": "canonical", "depth": 3,
		"ignored": "include", "hidden": "exclude", "type": "text",
		"stats":   []interface{}{"count", "total-size"},
		"sort_by": []interface{}{"format", "count"}, "sort_order": "asc",
	}
	if !reflect.DeepEqual(kwargs, want) {
		t.Errorf("Overlay result = %v, want %v", kwargs, want)
	}
}

func TestLoadOverlayOnlySetKeys(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	path := writeConfig(t, `method = "ext"`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Has("method") {
		t.Error("Has(method) = false, want true")
	}
	if cfg.Has("depth") || cfg.Has("exclude") {
		t.Error("Has reports unset keys as set")
	}
	kwargs := map[string]interface{}{"method": "hybrid", "depth": -1}
	cfg.Overlay(kwargs)
	if kwargs["method"] != "ext" {
		t.Errorf("method = %v, want ext", kwargs["method"])
	}
	if kwargs["depth"] != -1 {
		t.Errorf("depth = %v, want untouched -1", kwargs["depth"])
	}
}

func TestLoadEmptyExclude(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	path := writeConfig(t, `exclude = []`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Has("exclude") {
		t.Fatal("Has(exclude) = false for exclude = []")
	}
	kwargs := map[string]interface{}{"exclude": []interface{}{".git"}}
	cfg.Overlay(kwargs)
	got := kwargs["exclude"].([]interface{})
	if len(got) != 0 {
		t.Errorf("exclude = %v, want empty (scan everything)", got)
	}
}

func TestLoadMissingFile(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	path := filepath.Join(t.TempDir(), "nope.toml")
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load succeeded on a missing file")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not report the path %q", err, path)
	}
}

func TestLoadMalformedTOML(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	msg := loadErr(t, "exclude = [\nmethod =")
	if !regexp.MustCompile(`dirstat\.toml: [1-9][0-9]*:[1-9][0-9]*: `).MatchString(msg) {
		t.Errorf("parse error %q missing the line:column position after the path", msg)
	}
	if !strings.Contains(msg, "dirstat.toml") {
		t.Errorf("parse error %q missing the file path", msg)
	}
}

func TestLoadUnknownKey(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	msg := loadErr(t, `bogus = 1`)
	if !strings.Contains(msg, `"bogus"`) {
		t.Errorf("error %q does not name the unknown key", msg)
	}
	for _, key := range AllowedKeys {
		if !strings.Contains(msg, key) {
			t.Errorf("error %q missing allowed key %q", msg, key)
		}
	}
}

func TestLoadRenderingKeys(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	// Rendering/output keys get a distinct message: they exist as flags but
	// are deliberately not allowed in a scan config file (R42).
	for key, val := range map[string]string{
		"colors": "true", "style": `"ascii"`, "output": `"json"`, "top": "5",
		"show": `"both"`, "combined": "true", "singletons": `"show"`,
		"list_no_ext": "true", "legend": "true", "human": "true",
	} {
		msg := loadErr(t, key+" = "+val)
		if !strings.Contains(msg, `"`+key+`"`) {
			t.Errorf("error %q does not name the key %q", msg, key)
		}
		if !strings.Contains(msg, "rendering") {
			t.Errorf("error %q for %q missing the rendering/output explanation", msg, key)
		}
		for _, allowed := range AllowedKeys {
			if !strings.Contains(msg, allowed) {
				t.Errorf("error %q missing allowed key %q", msg, allowed)
			}
		}
	}
}

func TestLoadWhereKey(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	msg := loadErr(t, `where = "/some/dir"`)
	if !strings.Contains(msg, `"where"`) {
		t.Errorf("error %q does not name the where key", msg)
	}
	if !strings.Contains(msg, "positional") {
		t.Errorf("error %q should direct to the positional argument", msg)
	}
}

func TestLoadWrongType(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	tests := []struct {
		content string
		key     string
	}{
		{`exclude = "str"`, "exclude"},
		{`exclude = [1, 2]`, "exclude"},
		{`depth = "deep"`, "depth"},
		{`depth = 2.5`, "depth"},
		{`method = 3`, "method"},
		{`stats = ["count", 7]`, "stats"},
		{`sort_by = true`, "sort_by"},
		{`sort_order = ["asc"]`, "sort_order"},
		{`[exclude]`, "exclude"},
	}
	for _, tc := range tests {
		msg := loadErr(t, tc.content)
		if !strings.Contains(msg, `"`+tc.key+`"`) {
			t.Errorf("config %q: error %q does not name key %q", tc.content, msg, tc.key)
		}
	}
}

func TestLoadInvalidChoice(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	tests := []struct {
		content string
		key     string
		bad     string
	}{
		{`method = "x"`, "method", "x"},
		{`ignored = "y"`, "ignored", "y"},
		{`hidden = "only"`, "hidden", "only"},
		{`type = "w"`, "type", "w"},
		{`sort_order = "descending"`, "sort_order", "descending"},
	}
	for _, tc := range tests {
		msg := loadErr(t, tc.content)
		if !strings.Contains(msg, `"`+tc.key+`"`) || !strings.Contains(msg, `"`+tc.bad+`"`) {
			t.Errorf("config %q: error %q should name key %q and value %q", tc.content, msg, tc.key, tc.bad)
		}
	}
}

func TestLoadInvalidStatAndSortNames(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	msg := loadErr(t, `stats = ["count", "bogus"]`)
	if !strings.Contains(msg, `"bogus"`) || !strings.Contains(msg, `"stats"`) {
		t.Errorf("error %q should name the invalid stat and the key", msg)
	}
	msg = loadErr(t, `sort_by = ["sizes"]`)
	if !strings.Contains(msg, `"sizes"`) || !strings.Contains(msg, `"sort_by"`) {
		t.Errorf("error %q should name the invalid sort column and the key", msg)
	}
	// "format" is valid for sort_by but not for stats.
	msg = loadErr(t, `stats = ["format"]`)
	if !strings.Contains(msg, `"format"`) {
		t.Errorf("error %q should reject format as a stat", msg)
	}
	path := writeConfig(t, `sort_by = ["format"]`)
	if _, err := Load(path); err != nil {
		t.Errorf("sort_by = [format] must be valid: %v", err)
	}
}

func TestLoadDuplicateValues(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	for _, content := range []string{
		`exclude = [".git", ".git"]`,
		`stats = ["count", "count"]`,
		`sort_by = ["format", "format"]`,
	} {
		msg := loadErr(t, content)
		if !strings.Contains(msg, "duplicate") {
			t.Errorf("config %q: error %q should report the duplicate", content, msg)
		}
	}
}

func TestExplicitKeys(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{"none", []string{"dirstat", "scan", "."}, nil},
		{"space form", []string{"dirstat", "scan", "--method", "ext"}, []string{"method"}},
		{"equals form", []string{"dirstat", "scan", "--method=ext"}, []string{"method"}},
		{"dashed flag maps to underscored key",
			[]string{"dirstat", "scan", "--sort-by", "count", "--sort-order=asc"},
			[]string{"sort_by", "sort_order"}},
		{"repeatable counted once",
			[]string{"dirstat", "scan", "--exclude", "a", "--exclude=b"},
			[]string{"exclude"}},
		{"prefix is not a match", []string{"dirstat", "scan", "--methodical", "x"}, nil},
		{"non-scan flags ignored",
			[]string{"dirstat", "scan", "--config", "f.toml", "--colors", "--top=3"},
			nil},
		{"all scan keys",
			[]string{"dirstat", "scan", "--exclude=a", "--method=ext", "--formats=canonical",
				"--depth=1", "--ignored=only", "--hidden=exclude", "--type=text",
				"--stats=count", "--sort-by=format", "--sort-order=asc"},
			[]string{"exclude", "method", "formats", "depth", "ignored", "hidden", "type",
				"stats", "sort_by", "sort_order"}},
	}
	for _, tc := range tests {
		got := ExplicitKeys(tc.argv)
		wantSet := map[string]bool{}
		for _, k := range tc.want {
			wantSet[k] = true
		}
		if !reflect.DeepEqual(got, wantSet) {
			t.Errorf("%s: ExplicitKeys(%v) = %v, want %v", tc.name, tc.argv, got, wantSet)
		}
	}
}

// TestLoadFormatsKey: the formats key is a scan-semantic setting like method,
// so a config file may carry it, with the same closed value set the flag
// declares.
func TestLoadFormatsKey(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	for _, value := range []string{"raw", "canonical"} {
		path := writeConfig(t, `formats = "`+value+`"`)
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load formats = %q: %v", value, err)
		}
		if !cfg.Has("formats") {
			t.Errorf("Has(formats) = false for formats = %q", value)
		}
		kwargs := map[string]interface{}{"formats": "raw"}
		cfg.Overlay(kwargs)
		if kwargs["formats"] != value {
			t.Errorf("Overlay set formats = %v, want %q", kwargs["formats"], value)
		}
	}

	msg := loadErr(t, `formats = "bogus"`)
	if !strings.Contains(msg, `"bogus"`) || !strings.Contains(msg, "raw") || !strings.Contains(msg, "canonical") {
		t.Errorf("error %q does not name the invalid value and the valid ones", msg)
	}
	if msg := loadErr(t, `formats = 1`); !strings.Contains(msg, "must be a string") {
		t.Errorf("error %q does not report the wrong type", msg)
	}
}
