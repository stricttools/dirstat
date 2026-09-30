// Package render produces the colored terminal output: summary section,
// format tables, legend, and the extensionless-file list.
package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/stricttools/dirstat/internal/config"
	"github.com/stricttools/dirstat/internal/scan"
)

// Options controls table rendering. All fields are presentation-only and
// never affect JSON output.
type Options struct {
	Show       string // "summary", "table", or "both"
	Combined   bool
	Collapse   bool // --singletons collapse
	SortBy     []string
	SortDesc   bool
	Top        int
	ListNoExt  bool
	Legend     bool
	Colors     bool // effective: flag AND stdout is a TTY
	Human      bool
	Style      string // "unicode" or "ascii"
	TypeFilter string
	Stats      []string // selected stat names in canonical column order
	Width      int      // terminal width (80 when colors are inactive)
	Theme      config.Theme
	Config     string // absolute path of the scan config file; "" = none (R46)
}

// borderSet is the table border charset for one style.
type borderSet struct {
	tl, tm, tr string // top-left, top-mid, top-right
	ml, mm, mr string // mid-left, mid-mid, mid-right
	bl, bm, br string // bottom-left, bottom-mid, bottom-right
	v, h       string // vertical, horizontal
	rule       string // heading rule character
}

var borders = map[string]borderSet{
	"unicode": {"┌", "┬", "┐", "├", "┼", "┤", "└", "┴", "┘", "│", "─", "─"},
	"ascii":   {"+", "+", "+", "+", "+", "+", "+", "+", "+", "|", "-", "-"},
}

// Render writes the full table-mode output for a scan result.
func Render(w io.Writer, res *scan.Result, opts Options) {
	p := newPalette(opts.Colors, opts.Theme)
	b := borders[opts.Style]

	if opts.Show != "table" {
		renderSummary(w, res, opts, p, b)
	}
	if opts.Show != "summary" {
		renderTables(w, res, opts, p, b)
	}
	if opts.ListNoExt {
		renderNoExtList(w, res, opts, p, b)
	}
	fmt.Fprintln(w)
}

func heading(w io.Writer, title string, opts Options, p palette, b borderSet) {
	fmt.Fprintf(w, "%s%s%s\n", p.bold, title, p.reset)
	n := min(50, opts.Width)
	fmt.Fprintf(w, "%s%s%s\n", p.border, strings.Repeat(b.rule, n), p.reset)
}

func renderSummary(w io.Writer, res *scan.Result, opts Options, p palette, b borderSet) {
	fmt.Fprintln(w)
	heading(w, "Summary", opts, p, b)
	s := res.Summary
	type entry struct {
		label string
		value string
	}
	var entries []entry
	if opts.Config != "" {
		// A config file in effect is announced as the first summary line (R46).
		entries = append(entries, entry{"Config", opts.Config})
	}
	entries = append(entries, []entry{
		{"Directories", formatCount(int64(s.Directories), opts.Human)},
		{"Files", formatCount(int64(s.Files), opts.Human)},
		{"Files without extension", formatCount(int64(s.FilesWithoutExtension), opts.Human)},
		{"Max depth", formatCount(int64(s.MaxDepth), false)},
		{"Symlinks (skipped)", formatCount(int64(s.Symlinks), opts.Human)},
		{"Executables", formatCount(int64(s.Executables), opts.Human)},
		{"Unique formats", formatCount(int64(s.UniqueFormats), opts.Human)},
		{"Files sniffed", formatCount(int64(s.FilesSniffed), opts.Human)},
		{"Unreadable (skipped)", formatCount(int64(s.Unreadable), opts.Human)},
	}...)
	for _, e := range entries {
		fmt.Fprintf(w, "  %s%s:%s %s%s%s\n", p.statLabel, e.label, p.reset, p.statValue, e.value, p.reset)
	}
	fmt.Fprintln(w)
}

// prepare applies the render-only group pipeline: singleton collapse, sort,
// then --top truncation. The input slice is not modified.
func prepare(groups []scan.Group, opts Options) []scan.Group {
	out := make([]scan.Group, len(groups))
	copy(out, groups)
	if opts.Collapse {
		out = scan.CollapseSingletons(out)
	}
	scan.SortGroups(out, opts.SortBy, opts.SortDesc)
	if opts.Top > 0 && len(out) > opts.Top {
		out = out[:opts.Top]
	}
	return out
}

func renderTables(w io.Writer, res *scan.Result, opts Options, p palette, b borderSet) {
	if len(res.Groups) == 0 {
		fmt.Fprintf(w, "%sNo files found.%s\n", p.dim, p.reset)
		return
	}

	if opts.Combined {
		groups := prepare(res.Groups, opts)
		heading(w, "Files by Format", opts, p, b)
		renderTable(w, groups, true, opts, p, b)
		if opts.Legend && opts.TypeFilter == scan.TypeBoth && p.enabled {
			fmt.Fprintln(w)
			fmt.Fprintf(w, "  %s■%s Text files  %s■%s Binary files\n",
				p.textFG, p.reset, p.binaryFG, p.reset)
		}
		return
	}

	var text, binary []scan.Group
	for _, g := range res.Groups {
		if g.Text {
			text = append(text, g)
		} else {
			binary = append(binary, g)
		}
	}
	first := true
	if len(text) > 0 {
		heading(w, "Text Files by Format", opts, p, b)
		renderTable(w, prepare(text, opts), true, opts, p, b)
		first = false
	}
	if len(binary) > 0 {
		if !first {
			fmt.Fprintln(w)
		}
		heading(w, "Binary Files by Format", opts, p, b)
		renderTable(w, prepare(binary, opts), false, opts, p, b)
	}
}

// headersFor returns the column headers for the selected stats. includeLOC
// is false for the binary table in split mode, which omits LOC columns.
func headersFor(stats []string, includeLOC bool) []string {
	titles := map[string]string{
		"count":      "Count",
		"total-size": "Total Size",
		"min-size":   "Min Size",
		"max-size":   "Max Size",
		"avg-size":   "Avg Size",
		"total-loc":  "Total LOC",
		"min-loc":    "Min LOC",
		"max-loc":    "Max LOC",
		"avg-loc":    "Avg LOC",
	}
	headers := []string{"Format"}
	for _, s := range stats {
		if scan.IsLOCStat(s) && !includeLOC {
			continue
		}
		headers = append(headers, titles[s])
	}
	return headers
}

// buildRow builds the cells for one group row. Shared between the combined
// and split tables (R26).
func buildRow(g scan.Group, stats []string, human bool, includeLOC bool) []string {
	cells := []string{g.Format}
	locCell := func(v int64) string {
		if !g.HasLOC {
			return "-"
		}
		return formatCount(v, human)
	}
	for _, s := range stats {
		switch s {
		case "count":
			cells = append(cells, formatCount(int64(g.Count), human))
		case "total-size":
			cells = append(cells, formatSize(g.TotalSize, human))
		case "min-size":
			cells = append(cells, formatSize(g.MinSize, human))
		case "max-size":
			cells = append(cells, formatSize(g.MaxSize, human))
		case "avg-size":
			cells = append(cells, formatSize(g.AvgSize, human))
		case "total-loc", "min-loc", "max-loc", "avg-loc":
			if !includeLOC {
				continue
			}
			switch s {
			case "total-loc":
				cells = append(cells, locCell(g.TotalLOC))
			case "min-loc":
				cells = append(cells, locCell(g.MinLOC))
			case "max-loc":
				cells = append(cells, locCell(g.MaxLOC))
			case "avg-loc":
				cells = append(cells, locCell(g.AvgLOC))
			}
		}
	}
	return cells
}

// pad right-pads s with spaces to the given terminal display width. Widths
// are measured in display cells via runewidth, not bytes: %-*s would pad by
// byte count and misalign multibyte format names (R28).
func pad(s string, width int) string {
	gap := width - runewidth.StringWidth(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

// truncateCell shortens a cell to fit width display cells, rune-safely and
// with a ".." marker (R28). MIME-type group names (anything containing a
// "/") are truncated from the left, because their distinguishing part is the
// subtype: cutting the tail renders every `application/*` group as the same
// unreadable `applicat..`. Every other name keeps its prefix.
func truncateCell(s string, width int) string {
	if runewidth.StringWidth(s) <= width {
		return s
	}
	const marker = ".."
	if !strings.Contains(s, "/") || width <= len(marker) {
		return runewidth.Truncate(s, width, marker)
	}
	keep := width - len(marker)
	runes := []rune(s)
	i, w := len(runes), 0
	for i > 0 {
		rw := runewidth.RuneWidth(runes[i-1])
		if w+rw > keep {
			break
		}
		w += rw
		i--
	}
	return marker + string(runes[i:])
}

func renderTable(w io.Writer, groups []scan.Group, includeLOC bool, opts Options, p palette, b borderSet) {
	headers := headersFor(opts.Stats, includeLOC)
	rows := make([][]string, len(groups))
	for i, g := range groups {
		rows[i] = buildRow(g, opts.Stats, opts.Human, includeLOC)
	}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = runewidth.StringWidth(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if cw := runewidth.StringWidth(cell); cw > widths[i] {
				widths[i] = cw
			}
		}
	}

	// Width adaptation: shrink only the Format column, floor 10 (R28).
	total := 1
	for _, cw := range widths {
		total += cw + 3
	}
	if total > opts.Width {
		widths[0] = max(10, widths[0]-(total-opts.Width))
	}

	line := func(l, m, r string) string {
		parts := make([]string, len(widths))
		for i, cw := range widths {
			parts[i] = strings.Repeat(b.h, cw+2)
		}
		return p.border + l + strings.Join(parts, m) + r + p.reset
	}

	fmt.Fprintln(w, line(b.tl, b.tm, b.tr))

	var sb strings.Builder
	sb.WriteString(p.border + b.v + p.reset)
	for i, h := range headers {
		sb.WriteString(fmt.Sprintf(" %s%s%s%s%s %s%s%s",
			p.headerFG, p.headerBG, p.bold, pad(h, widths[i]), p.reset, p.border, b.v, p.reset))
	}
	fmt.Fprintln(w, sb.String())
	fmt.Fprintln(w, line(b.ml, b.mm, b.mr))

	for ri, row := range rows {
		g := groups[ri]
		var fg, bg string
		switch {
		case g.Pseudo:
			fg = p.headerFG // neutral color for the (singletons) row
		case g.Text:
			fg, bg = p.textFG, p.textBG
		default:
			fg, bg = p.binaryFG, p.binaryBG
		}
		sb.Reset()
		sb.WriteString(p.border + b.v + p.reset)
		for i, cell := range row {
			cell = truncateCell(cell, widths[i])
			sb.WriteString(fmt.Sprintf(" %s%s%s%s %s%s%s",
				fg, bg, pad(cell, widths[i]), p.reset, p.border, b.v, p.reset))
		}
		fmt.Fprintln(w, sb.String())
	}

	fmt.Fprintln(w, line(b.bl, b.bm, b.br))
}

func renderNoExtList(w io.Writer, res *scan.Result, opts Options, p palette, b borderSet) {
	fmt.Fprintln(w)
	heading(w, "Files without extension", opts, p, b)
	if len(res.NoExtFiles) == 0 {
		fmt.Fprintf(w, "  %s(none)%s\n", p.statLabel, p.reset)
		return
	}
	for _, f := range res.NoExtFiles {
		fmt.Fprintf(w, "  %s%s%s\n", p.statValue, f, p.reset)
	}
}
