package test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stricttools/dirstat/internal/testutil"
	"github.com/stricttools/testisolation/go/hygiene"
)

// The machine-output surface is the framework's: `--json` enters machine mode,
// stdout carries exactly one document (the envelope), and the scan document is
// its `payload` member, validated against the command's declared schema. The
// old `--output json` spelling is gone with no shim.

func machineTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"a.go":      "package main\n\nfunc main() {}\n",
		"README.md": "# Title\n",
		"data.bin":  "\x00\x01\x02\x03",
	})
	return root
}

func TestMachineModeStdoutIsTheEnvelope(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := machineTree(t)
	stdout, stderr, code := runDirstat(t, "scan", root, "--json")
	if code != 0 {
		t.Fatalf("dirstat exited %d: %s", code, stderr)
	}

	var env map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\n%s", err, stdout)
	}
	if env["app"] != "dirstat" {
		t.Errorf("app = %v, want dirstat", env["app"])
	}
	if env["command"] != "scan" {
		t.Errorf("command = %v, want scan", env["command"])
	}
	if env["exit_code"] != float64(0) {
		t.Errorf("exit_code = %v, want 0", env["exit_code"])
	}
	if v, ok := env["app_version"].(string); !ok || v == "" {
		t.Errorf("app_version missing or empty: %v", env["app_version"])
	}
	if _, ok := env["interface_version"]; !ok {
		t.Error("envelope has no interface_version")
	}

	// The human table never reaches stdout in machine mode.
	if strings.Contains(stdout, "Directories") || strings.Contains(stdout, "│") {
		t.Errorf("table rendering leaked into machine-mode stdout:\n%s", stdout)
	}
}

func TestMachineModePayloadCarriesTheScanDocument(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := machineTree(t)
	parsed := scanJSON(t, "scan", root, "--json", "--list-no-ext")

	if parsed["root"] == nil || parsed["method"] != "hybrid" {
		t.Errorf("payload lacks root/method: %v", parsed)
	}
	if got := summaryField(t, parsed, "files"); got != 3 {
		t.Errorf("summary.files = %d, want 3", got)
	}
	if _, ok := parsed["groups"].([]interface{}); !ok {
		t.Errorf("payload has no groups array: %v", parsed)
	}
	if _, ok := parsed["no_extension_files"]; !ok {
		t.Error("--list-no-ext must add no_extension_files to the payload")
	}
	// The envelope carries the version as app_version; the payload does not
	// restate it.
	if _, present := parsed["dirstat_version"]; present {
		t.Error("payload must not carry dirstat_version; the envelope's app_version is the one place")
	}
}

func TestOutputFlagIsGone(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := machineTree(t)
	_, stderr, code := runDirstat(t, "scan", root, "--output", "json")
	if code == 0 {
		t.Fatal("--output must be an unknown flag now")
	}
	if !strings.Contains(stderr, "output") {
		t.Errorf("error should name the unknown flag: %s", stderr)
	}
}

func TestHumanModeStillRendersTheTable(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := machineTree(t)
	stdout, stderr, code := runDirstat(t, "scan", root)
	if code != 0 {
		t.Fatalf("dirstat exited %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Directories") {
		t.Errorf("human mode lost its summary section:\n%s", stdout)
	}
	if strings.Contains(stdout, `"interface_version"`) {
		t.Errorf("the envelope must never appear outside machine mode:\n%s", stdout)
	}
}
