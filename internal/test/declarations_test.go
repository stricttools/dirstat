package test

import (
	"strings"
	"testing"

	"github.com/smm-h/dirstat/internal/testutil"
	"github.com/stricttools/testisolation/go/hygiene"
)

// The declaration regime (strictcli contract §23, §24.9): every flag and the
// positional argument declare exactly one presence, and every choices entry is
// a record carrying its own help. `scan` is read_only, so a declared default is
// legal on it and most flags keep one; the two declarations that changed shape
// are pinned here, together with the machine surface a documentation pipeline
// reads out of this command.

// TestStatsAbsenceSelectsEveryStat: --stats is optional rather than defaulted
// to a null value, and an omitted --stats still means every stat, while a
// supplied one still means exactly what was named.
func TestStatsAbsenceSelectsEveryStat(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{"a.go": "package main\n"})

	every := scanJSON(t, "scan", root, "--json")
	group := firstGroup(t, every)
	for _, key := range []string{"count", "total_size", "min_size", "max_size",
		"avg_size", "total_loc", "min_loc", "max_loc", "avg_loc"} {
		if _, ok := group[key]; !ok {
			t.Errorf("an omitted --stats must select every stat; %q is missing", key)
		}
	}

	named := scanJSON(t, "scan", root, "--json", "--stats", "count")
	group = firstGroup(t, named)
	if _, ok := group["count"]; !ok {
		t.Error("--stats count must select count")
	}
	if _, ok := group["total_loc"]; ok {
		t.Error("--stats count must select nothing else")
	}
}

// TestWhereDefaultsToTheCurrentDirectory: the positional argument declares its
// presence as a default, so an omitted target is the current directory rather
// than a missing-argument refusal.
func TestWhereDefaultsToTheCurrentDirectory(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	_, stderr, code := runDirstat(t, "scan", "--json")
	if code != 0 {
		t.Fatalf("an omitted target must scan the current directory; exited %d: %s", code, stderr)
	}
}

// TestChoiceEntriesCarryTheirOwnHelp: every closed-set flag declares its values
// as records, so --help describes each value beneath the flag instead of the
// flag's own prose restating the list.
func TestChoiceEntriesCarryTheirOwnHelp(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	stdout, stderr, code := runDirstat(t, "scan", "--help")
	if code != 0 {
		t.Fatalf("scan --help exited %d: %s", code, stderr)
	}
	for _, want := range []string{
		"content-sniff every file",
		"keep only the paths git ignores",
		"skip dot-prefixed entries",
		"keep the text groups",
		"smallest first",
		"the summary section alone",
		"one (singletons) row standing for all of them",
		"plain ASCII borders",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("scan --help does not describe a choice value: %q", want)
		}
	}
	// The one-line form is what a helpless choices list renders; no flag
	// should still be in it.
	if strings.Contains(stdout, "[choices:") {
		t.Errorf("a choices flag still renders the one-line form:\n%s", stdout)
	}
}

// TestSourceCountingContract pins the machine surface a documentation pipeline
// reads: the exact invocation it makes, and the three payload fields it takes
// from every group. The envelope's own members are the framework's; what dirstat
// owes a consumer is that this argv answers with a payload carrying these
// groups.
func TestSourceCountingContract(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{
		"a.go":     "package main\n\nfunc main() {}\n",
		"b.go":     "package main\n",
		"data.bin": "\x00\x01\x02\x03",
	})

	payload := scanJSON(t, "scan", root, "--json",
		"--type", "text", "--stats", "count", "--stats", "total-loc")

	groups, ok := payload["groups"].([]interface{})
	if !ok || len(groups) == 0 {
		t.Fatalf("payload carries no groups array: %v", payload)
	}
	seen := false
	for _, raw := range groups {
		group, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("a group is not an object: %v", raw)
		}
		format, ok := group["format"].(string)
		if !ok {
			t.Errorf("group has no format string: %v", group)
			continue
		}
		if _, ok := group["count"].(float64); !ok {
			t.Errorf("group %q has no numeric count: %v", format, group)
		}
		if _, ok := group["total_loc"].(float64); !ok {
			t.Errorf("group %q has no numeric total_loc: %v", format, group)
		}
		if format == "bin" {
			t.Errorf("--type text must not answer with the binary group: %v", group)
		}
		if format == "go" {
			seen = true
			if got := group["count"].(float64); got != 2 {
				t.Errorf("go count = %v, want 2", got)
			}
			if got := group["total_loc"].(float64); got != 4 {
				t.Errorf("go total_loc = %v, want 4", got)
			}
		}
	}
	if !seen {
		t.Error("the go group is missing from a text-only scan")
	}
}

// firstGroup returns the first group object of a scan payload.
func firstGroup(t *testing.T, payload map[string]interface{}) map[string]interface{} {
	t.Helper()
	groups, ok := payload["groups"].([]interface{})
	if !ok || len(groups) == 0 {
		t.Fatalf("payload carries no groups: %v", payload)
	}
	group, ok := groups[0].(map[string]interface{})
	if !ok {
		t.Fatalf("first group is not an object: %v", groups[0])
	}
	return group
}
