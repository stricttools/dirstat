package jsonout

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stricttools/dirstat/internal/scan"
	"github.com/stricttools/testisolation/go/hygiene"
)

func fullSelection() Selection {
	sel := Selection{}
	for _, s := range scan.StatNames {
		sel[s] = true
	}
	return sel
}

func testResult() *scan.Result {
	return &scan.Result{
		Root:   "/data/project",
		Method: "hybrid",
		Summary: scan.Summary{
			Directories: 3, Files: 5, FilesWithoutExtension: 1, MaxDepth: 2,
			Symlinks: 1, Executables: 2, UniqueFormats: 2, FilesSniffed: 1, Unreadable: 0,
		},
		Groups: []scan.Group{
			{Format: "go", Text: true, Count: 4, TotalSize: 400, MinSize: 50, MaxSize: 200, AvgSize: 100,
				HasLOC: true, TotalLOC: 40, MinLOC: 2, MaxLOC: 20, AvgLOC: 10},
			{Format: "png", Text: false, Count: 1, TotalSize: 9000, MinSize: 9000, MaxSize: 9000, AvgSize: 9000},
		},
		NoExtFiles: []string{"Makefile"},
	}
}

// marshal renders a built document the way the framework renders it inside the
// envelope: encoding/json over the same value.
func marshal(t *testing.T, d Document) string {
	t.Helper()
	out, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func buildToString(t *testing.T, res *scan.Result, sel Selection, listNoExt bool) string {
	t.Helper()
	return marshal(t, Build("", res, res.Groups, sel, listNoExt))
}

func TestBuildConfigField(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	// With a config path: additive "config" field directly after "method" (R46).
	out := marshal(t, Build("/etc/dirstat.toml", testResult(), nil, fullSelection(), false))
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if parsed["config"] != "/etc/dirstat.toml" {
		t.Errorf("config = %v, want /etc/dirstat.toml", parsed["config"])
	}
	method := strings.Index(out, `"method"`)
	config := strings.Index(out, `"config"`)
	summary := strings.Index(out, `"summary"`)
	if !(method < config && config < summary) {
		t.Errorf("config field must sit between method and summary:\n%s", out)
	}

	// Without a config path the field is absent entirely (additive schema, R34).
	out = buildToString(t, testResult(), fullSelection(), false)
	if strings.Contains(out, `"config"`) {
		t.Errorf("config field must be omitted when --config is unused:\n%s", out)
	}
}

func TestBuildFullSchema(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	res := testResult()
	out := buildToString(t, res, fullSelection(), true)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if parsed["root"] != "/data/project" || parsed["method"] != "hybrid" {
		t.Errorf("bad top-level fields: %v", parsed)
	}
	// The binary's version is the envelope's app_version, never a payload field.
	if _, present := parsed["dirstat_version"]; present {
		t.Error("the document must not restate the binary version")
	}
	summary := parsed["summary"].(map[string]interface{})
	for key, want := range map[string]float64{
		"directories": 3, "files": 5, "files_without_extension": 1, "max_depth": 2,
		"symlinks": 1, "executables": 2, "unique_formats": 2, "files_sniffed": 1, "unreadable": 0,
	} {
		if summary[key] != want {
			t.Errorf("summary[%s] = %v, want %v", key, summary[key], want)
		}
	}

	groups := parsed["groups"].([]interface{})
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	goGroup := groups[0].(map[string]interface{})
	if goGroup["format"] != "go" || goGroup["text"] != true || goGroup["total_loc"] != float64(40) {
		t.Errorf("bad go group: %v", goGroup)
	}
	pngGroup := groups[1].(map[string]interface{})
	if pngGroup["total_loc"] != nil {
		t.Errorf("binary group total_loc should be null, got %v", pngGroup["total_loc"])
	}
	if v, present := pngGroup["min_loc"]; !present || v != nil {
		t.Errorf("binary group min_loc should be present and null, got %v (present=%v)", v, present)
	}

	noExt := parsed["no_extension_files"].([]interface{})
	if len(noExt) != 1 || noExt[0] != "Makefile" {
		t.Errorf("bad no_extension_files: %v", noExt)
	}

	// No ANSI codes ever.
	if strings.Contains(out, "\033") {
		t.Error("JSON output contains ANSI escape")
	}
}

func TestBuildFieldOrder(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	out := buildToString(t, testResult(), fullSelection(), false)
	wantOrder := []string{
		`"root"`, `"method"`, `"summary"`,
		`"directories"`, `"files"`, `"files_without_extension"`, `"max_depth"`,
		`"symlinks"`, `"executables"`, `"unique_formats"`, `"files_sniffed"`, `"unreadable"`,
		`"groups"`, `"format"`, `"text"`, `"count"`,
		`"total_size"`, `"min_size"`, `"max_size"`, `"avg_size"`,
		`"total_loc"`, `"min_loc"`, `"max_loc"`, `"avg_loc"`,
	}
	pos := -1
	for _, key := range wantOrder {
		i := strings.Index(out, key)
		if i < 0 {
			t.Fatalf("missing key %s:\n%s", key, out)
		}
		if i < pos {
			t.Errorf("key %s out of order", key)
		}
		pos = i
	}
}

func TestBuildStatsSelection(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	sel := Selection{"count": true, "total-size": true}
	out := buildToString(t, testResult(), sel, false)
	for _, absent := range []string{`"min_size"`, `"avg_size"`, `"total_loc"`, `"avg_loc"`} {
		if strings.Contains(out, absent) {
			t.Errorf("unselected stat %s present in output:\n%s", absent, out)
		}
	}
	for _, present := range []string{`"count"`, `"total_size"`, `"format"`, `"text"`} {
		if !strings.Contains(out, present) {
			t.Errorf("expected %s in output:\n%s", present, out)
		}
	}
}

func TestBuildNoExtOmittedAndEmpty(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	res := testResult()
	out := buildToString(t, res, fullSelection(), false)
	if strings.Contains(out, "no_extension_files") {
		t.Error("no_extension_files must be omitted without --list-no-ext")
	}

	res.NoExtFiles = nil
	out = buildToString(t, res, fullSelection(), true)
	if !strings.Contains(out, `"no_extension_files":[]`) {
		t.Errorf("expected empty array for no_extension_files:\n%s", out)
	}
}

func TestBuildEmptyGroups(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	res := &scan.Result{Root: "/x", Method: "ext"}
	out := buildToString(t, res, fullSelection(), false)
	if !strings.Contains(out, `"groups":[]`) {
		t.Errorf("expected empty groups array:\n%s", out)
	}
}
