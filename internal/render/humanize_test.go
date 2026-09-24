package render

import (
	"testing"

	"github.com/stricttools/testisolation/go/hygiene"
)

func TestFormatSize(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	tests := []struct {
		n     int64
		human bool
		want  string
	}{
		{0, true, "0B"},
		{512, true, "512B"},
		{1023, true, "1023B"},
		{1024, true, "1.0KB"},
		{1536, true, "1.5KB"},
		{1048576, true, "1.0MB"},
		{5 * 1024 * 1024 * 1024, true, "5.0GB"},
		{1536, false, "1536"},
		{0, false, "0"},
	}
	for _, tc := range tests {
		if got := formatSize(tc.n, tc.human); got != tc.want {
			t.Errorf("formatSize(%d, %v) = %q, want %q", tc.n, tc.human, got, tc.want)
		}
	}
}

func TestFormatCount(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	tests := []struct {
		n     int64
		human bool
		want  string
	}{
		{0, true, "0"},
		{999, true, "999"},
		{1000, true, "1,000"},
		{1234567, true, "1,234,567"},
		{1234567, false, "1234567"},
		{-1234, true, "-1,234"},
	}
	for _, tc := range tests {
		if got := formatCount(tc.n, tc.human); got != tc.want {
			t.Errorf("formatCount(%d, %v) = %q, want %q", tc.n, tc.human, got, tc.want)
		}
	}
}
