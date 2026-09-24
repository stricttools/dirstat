package scan

import (
	"github.com/stricttools/testisolation/go/hygiene"
	"reflect"
	"testing"
)

func mkGroup(format string, text bool, count int, totalSize, totalLOC int64) Group {
	g := Group{
		Format: format, Text: text, Count: count,
		TotalSize: totalSize, MinSize: totalSize, MaxSize: totalSize,
		AvgSize: roundDiv(totalSize, int64(count)),
	}
	if text {
		g.HasLOC = true
		g.TotalLOC = totalLOC
		g.MinLOC = totalLOC
		g.MaxLOC = totalLOC
		g.AvgLOC = roundDiv(totalLOC, int64(count))
	}
	return g
}

func formats(groups []Group) []string {
	out := make([]string, len(groups))
	for i, g := range groups {
		out[i] = g.Format
	}
	return out
}

func TestSortGroups(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	base := []Group{
		mkGroup("go", true, 5, 500, 100),
		mkGroup("png", false, 5, 900, 0),
		mkGroup("md", true, 2, 100, 50),
		mkGroup("BIN", false, 2, 100, 0),
	}
	tests := []struct {
		name string
		keys []string
		desc bool
		want []string
	}{
		{"count desc, tie by format asc", []string{"count"}, true, []string{"go", "png", "BIN", "md"}},
		{"count asc", []string{"count"}, false, []string{"BIN", "md", "go", "png"}},
		{"format asc is case-insensitive", []string{"format"}, false, []string{"BIN", "go", "md", "png"}},
		{"format desc", []string{"format"}, true, []string{"png", "md", "go", "BIN"}},
		{"multi-key: count then total-size desc", []string{"count", "total-size"}, true, []string{"png", "go", "BIN", "md"}},
		{"loc key: binary sorts as 0", []string{"total-loc"}, true, []string{"go", "md", "BIN", "png"}},
	}
	for _, tc := range tests {
		groups := make([]Group, len(base))
		copy(groups, base)
		SortGroups(groups, tc.keys, tc.desc)
		if got := formats(groups); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCollapseSingletons(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	groups := []Group{
		mkGroup("go", true, 3, 300, 90),
		mkGroup("md", true, 1, 10, 4),
		mkGroup("txt", true, 1, 30, 8),
		mkGroup("png", false, 1, 1000, 0),
	}
	out := CollapseSingletons(groups)
	if len(out) != 2 {
		t.Fatalf("expected 2 groups after collapse, got %d: %v", len(out), formats(out))
	}
	var merged *Group
	for i := range out {
		if out[i].Format == SingletonsGroup {
			merged = &out[i]
		}
	}
	if merged == nil {
		t.Fatalf("no %s group in %v", SingletonsGroup, formats(out))
	}
	if !merged.Pseudo {
		t.Error("merged group must be marked Pseudo")
	}
	if merged.Count != 3 {
		t.Errorf("merged count = %d, want 3 (number of merged groups)", merged.Count)
	}
	if merged.TotalSize != 1040 || merged.MinSize != 10 || merged.MaxSize != 1000 {
		t.Errorf("merged sizes = total %d min %d max %d, want 1040/10/1000",
			merged.TotalSize, merged.MinSize, merged.MaxSize)
	}
	if merged.AvgSize != roundDiv(1040, 3) {
		t.Errorf("merged avg size = %d, want %d", merged.AvgSize, roundDiv(1040, 3))
	}
	// LOC aggregates over text singletons only (md, txt).
	if !merged.HasLOC || merged.TotalLOC != 12 || merged.MinLOC != 4 || merged.MaxLOC != 8 || merged.AvgLOC != 6 {
		t.Errorf("merged LOC = hasLOC %v total %d min %d max %d avg %d, want true/12/4/8/6",
			merged.HasLOC, merged.TotalLOC, merged.MinLOC, merged.MaxLOC, merged.AvgLOC)
	}
}

func TestCollapseSingletonsNoSingles(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	groups := []Group{mkGroup("go", true, 3, 300, 90)}
	out := CollapseSingletons(groups)
	if !reflect.DeepEqual(out, groups) {
		t.Errorf("collapse without singletons should be a no-op, got %v", formats(out))
	}
}

func TestCollapseSingletonsAllBinary(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	groups := []Group{
		mkGroup("png", false, 1, 100, 0),
		mkGroup("jpg", false, 1, 200, 0),
	}
	out := CollapseSingletons(groups)
	if len(out) != 1 || out[0].Format != SingletonsGroup {
		t.Fatalf("expected single merged group, got %v", formats(out))
	}
	if out[0].HasLOC {
		t.Error("merged group of binary singletons must not have LOC")
	}
}

func TestRoundDiv(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	tests := []struct {
		total, count, want int64
	}{
		{10, 3, 3}, // 3.33 -> 3
		{11, 3, 4}, // 3.67 -> 4
		{3, 2, 2},  // 1.5 -> 2 (rounded, not floored)
		{0, 5, 0},
		{7, 7, 1},
	}
	for _, tc := range tests {
		if got := roundDiv(tc.total, tc.count); got != tc.want {
			t.Errorf("roundDiv(%d, %d) = %d, want %d", tc.total, tc.count, got, tc.want)
		}
	}
}
