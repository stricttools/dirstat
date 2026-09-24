package test

import (
	"fmt"
	"github.com/stricttools/testisolation/go/hygiene"
	"strings"
	"testing"

	"github.com/smm-h/dirstat/internal/testutil"
)

// TestDeterminism verifies that repeated runs over the same tree produce
// byte-identical output despite the parallel worker pool (R39).
func TestDeterminism(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	root := t.TempDir()
	tree := make(map[string]string)
	for i := range 120 {
		tree[fmt.Sprintf("pkg%d/file%d.go", i%7, i)] = strings.Repeat("line\n", i+1)
		tree[fmt.Sprintf("pkg%d/noext%d", i%7, i)] = fmt.Sprintf("plain text %d\n", i)
		tree[fmt.Sprintf("bin/blob%d.bin", i)] = "\x00\x01\x02" + strings.Repeat("x", i)
	}
	testutil.WriteTree(t, root, tree)

	for _, args := range [][]string{
		{"scan", root},
		{"scan", root, "--json", "--list-no-ext"},
		{"scan", root, "--no-combined", "--singletons", "collapse", "--sort-by", "total-loc"},
	} {
		first, stderr, code := runDirstat(t, args...)
		if code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, stderr)
		}
		second, stderr, code := runDirstat(t, args...)
		if code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, stderr)
		}
		if first != second {
			t.Errorf("%v: two runs produced different output", args)
		}
	}
}
