package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stricttools/dirstat/internal/config"
	"github.com/stricttools/dirstat/internal/jsonout"
	"github.com/stricttools/dirstat/internal/render"
	"github.com/stricttools/dirstat/internal/scan"
	"github.com/stricttools/dirstat/internal/scanconfig"
	"github.com/stricttools/strictcli/go/strictcli"
	"golang.org/x/term"
)

// defaultExcludes is the curated built-in default of --exclude (R45):
// common VCS, dependency, build-output, cache, and IDE directories. Any
// explicit --exclude or a config-file exclude key replaces it entirely; there
// is no additive merging anywhere.
var defaultExcludes = []interface{}{
	".git", "node_modules", ".venv", "vendor", "build", "dist", "target",
	"zig-out", "zig-pkg", ".next", ".svelte-kit", "__pycache__",
	".mypy_cache", ".ruff_cache", ".pytest_cache", ".hypothesis",
	".gradle", ".idea", ".wrangler", ".vscode",
}

func registerScanCmd(app *strictcli.App) {
	app.Command("scan", "Summarize the files under a directory tree, grouped by format, with counts, sizes, and lines of code as terminal tables or JSON.", handleScan,
		// read_only: scan walks the tree, reads file contents to classify and
		// count them, and writes only to stdout/stderr. It creates, modifies
		// and deletes nothing.
		strictcli.WithEffect(strictcli.EffectReadOnly),
		strictcli.WithFlags(
			strictcli.StringFlag("method",
				"how each scanned file's format group is decided before it is counted",
				strictcli.Choices(
					strictcli.Ch("ext", "group by extension only, sniffing nothing"),
					strictcli.Ch("type", "content-sniff every file"),
					strictcli.Ch("hybrid", "trust known text extensions, content-sniff everything else"),
				), strictcli.Default("hybrid")),
			strictcli.StringFlag("formats",
				"how the chosen group's name is spelled before files are counted under it",
				strictcli.Choices(
					strictcli.Ch("raw", "the extension or sniffed MIME type exactly as it came out"),
					strictcli.Ch("canonical", "merge alias formats (mjs into js, h into c) and name extensionless scripts by their shebang interpreter"),
				), strictcli.Default("raw")),
			strictcli.IntFlag("depth",
				"maximum directory depth below the root; -1 = unlimited; the root is depth 0",
				strictcli.Default(-1)),
			strictcli.StringFlag("exclude",
				"exact base name to skip, matching directories and files; repeatable; passing the flag replaces the built-in default list entirely",
				strictcli.Repeatable(), strictcli.Unique(true), strictcli.Default(defaultExcludes)),
			strictcli.StringFlag("config",
				"path to a TOML scan-config file (keys: exclude, method, formats, depth, ignored, hidden, type, stats, sort_by, sort_order); never auto-discovered; a key set both in the file and on the command line is an error",
				strictcli.Default("")),
			strictcli.StringFlag("ignored",
				"gitignored-path handling; outside a git work tree, exclude and only behave as if no patterns exist",
				strictcli.Choices(
					strictcli.Ch("include", "no gitignore matching at all"),
					strictcli.Ch("exclude", "drop the paths git ignores"),
					strictcli.Ch("only", "keep only the paths git ignores"),
				), strictcli.Default("exclude")),
			strictcli.StringFlag("hidden",
				"dot-prefixed entry handling; the root directory itself is never treated as hidden",
				strictcli.Choices(
					strictcli.Ch("include", "walk into dot-prefixed entries"),
					strictcli.Ch("exclude", "skip dot-prefixed entries"),
				), strictcli.Default("include")),
			// Optional rather than an empty-list default: absence here is not an
			// empty selection, it is the unstated one, and the handler reads it
			// as every stat.
			strictcli.StringFlag("stats",
				"stat to compute and show (repeatable): count, total-size, min-size, max-size, avg-size, total-loc, min-loc, max-loc, avg-loc; omitted, every stat is shown",
				strictcli.Repeatable(), strictcli.Unique(true), strictcli.Optional()),
			strictcli.StringFlag("type",
				"filter the reported groups by their text/binary classification",
				strictcli.Choices(
					strictcli.Ch("text", "keep the text groups"),
					strictcli.Ch("binary", "keep the binary groups"),
					strictcli.Ch("both", "keep every group"),
				), strictcli.Default("both")),
			strictcli.StringFlag("sort-by",
				"sort column, in precedence order when repeated: format, count, total-size, min-size, max-size, avg-size, total-loc, min-loc, max-loc, avg-loc",
				strictcli.Repeatable(), strictcli.Unique(true), strictcli.Default([]interface{}{"count"})),
			strictcli.StringFlag("sort-order",
				"sort direction, applied uniformly to every --sort-by column",
				strictcli.Choices(
					strictcli.Ch("asc", "smallest first"),
					strictcli.Ch("desc", "largest first"),
				), strictcli.Default("desc")),
			strictcli.IntFlag("top",
				"keep only the first N groups after sorting (table output only); -1 or 0 = all",
				strictcli.Default(-1)),
			strictcli.StringFlag("show",
				"which sections the human table output renders; ignored in machine mode",
				strictcli.Choices(
					strictcli.Ch("summary", "the summary section alone"),
					strictcli.Ch("table", "the format tables alone"),
					strictcli.Ch("both", "the summary and the tables"),
				), strictcli.Default("both")),
			strictcli.BoolFlag("combined",
				"render one merged table with text and binary rows distinguished by color, instead of separate text/binary tables",
				strictcli.Default(true)),
			strictcli.StringFlag("singletons",
				"how one-file formats are rendered (table output only)",
				strictcli.Choices(
					strictcli.Ch("show", "one row per one-file format, as-is"),
					strictcli.Ch("collapse", "one (singletons) row standing for all of them"),
				), strictcli.Default("show")),
			strictcli.BoolFlag("list-no-ext",
				"list the paths of extensionless files (a section in table output, a field in JSON output)",
				strictcli.Default(false)),
			strictcli.BoolFlag("legend",
				"show the text/binary color legend under the combined table (needs active colors)",
				strictcli.Default(true)),
			strictcli.BoolFlag("colors",
				"use ANSI colors; auto-disabled when stdout is not a TTY",
				strictcli.Default(true)),
			strictcli.BoolFlag("human",
				"human-readable sizes and thousands separators (table output only)",
				strictcli.Default(true)),
			strictcli.StringFlag("style",
				"which character set the human table's borders are drawn with",
				strictcli.Choices(
					strictcli.Ch("unicode", "unicode box-drawing borders"),
					strictcli.Ch("ascii", "plain ASCII borders"),
				), strictcli.Default("unicode")),
		),
		strictcli.WithArgs(
			// The default IS the presence declaration: an omitted target scans
			// the current directory.
			strictcli.NewArg("where", "directory to summarize; omitted, the current directory",
				strictcli.ArgDefault(".")),
		),
		// Machine output is the framework's --json, and the scan document is
		// this command's payload. dirstat declares no output-format flag of
		// its own: the enum's other value was the human table, which is what
		// the command does when it is not asked for a document.
		strictcli.PayloadSchema(jsonout.Schema),
	)
}

// errorf prints a handler error line to stderr. The "error:" prefix is
// colored with the theme's error color when stderr is a TTY and --colors is
// true; stderr coloring is independent of --output mode (R30).
func errorf(colors bool, format string, args ...interface{}) {
	colored := colors && term.IsTerminal(int(os.Stderr.Fd()))
	fmt.Fprint(os.Stderr, render.FormatError(colored, config.LoadTheme(render.DetectThemeName()), format, args...))
}

// stringList extracts a repeatable string flag's values.
func stringList(kwargs map[string]interface{}, key string) []string {
	raw, _ := kwargs[key].([]interface{})
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		out = append(out, v.(string))
	}
	return out
}

func handleScan(ctx *strictcli.Context, kwargs map[string]interface{}) strictcli.Outcome {
	colorsFlag := kwargs["colors"].(bool)

	where := kwargs["where"].(string)
	root, err := filepath.Abs(where)
	if err != nil {
		errorf(colorsFlag, "resolving path %q: %s", where, err)
		return strictcli.Exit(ExitUsage)
	}
	info, err := os.Stat(root)
	if err != nil {
		errorf(colorsFlag, "directory does not exist: %s", root)
		return strictcli.Exit(ExitUsage)
	}
	if !info.IsDir() {
		errorf(colorsFlag, "path is not a directory: %s", root)
		return strictcli.Exit(ExitUsage)
	}

	// Scan config file: loaded and fully validated before any scanning
	// starts (R41). File values overlay the flag defaults in kwargs; a key
	// set both in the file and explicitly on the command line is a hard
	// error, so no silent override can occur (R43, R44).
	var configAbs string
	if configPath := kwargs["config"].(string); configPath != "" {
		cfg, err := scanconfig.Load(configPath)
		if err != nil {
			errorf(colorsFlag, "%s", err)
			return strictcli.Exit(ExitUsage)
		}
		explicit := scanconfig.ExplicitKeys(os.Args)
		for _, key := range scanconfig.AllowedKeys {
			if cfg.Has(key) && explicit[key] {
				errorf(colorsFlag, "key %q is set in the config file and --%s was passed on the command line; use one or the other",
					key, strings.ReplaceAll(key, "_", "-"))
				return strictcli.Exit(ExitUsage)
			}
		}
		cfg.Overlay(kwargs)
		if configAbs, err = filepath.Abs(configPath); err != nil {
			errorf(colorsFlag, "resolving config path %q: %s", configPath, err)
			return strictcli.Exit(ExitUsage)
		}
	}

	// Validate --stats (empty = all).
	validStats := make(map[string]bool, len(scan.StatNames))
	for _, s := range scan.StatNames {
		validStats[s] = true
	}
	statsRaw := stringList(kwargs, "stats")
	selected := make(jsonout.Selection)
	if len(statsRaw) == 0 {
		for _, s := range scan.StatNames {
			selected[s] = true
		}
	} else {
		for _, s := range statsRaw {
			if !validStats[s] {
				errorf(colorsFlag, "invalid stat %q; valid: %v", s, scan.StatNames)
				return strictcli.Exit(ExitUsage)
			}
			selected[s] = true
		}
	}
	// Selected stats in canonical column order.
	var statsOrdered []string
	for _, s := range scan.StatNames {
		if selected[s] {
			statsOrdered = append(statsOrdered, s)
		}
	}

	// Validate --sort-by.
	sortBy := stringList(kwargs, "sort_by")
	if len(sortBy) == 0 {
		sortBy = []string{"count"}
	}
	for _, s := range sortBy {
		if s != "format" && !validStats[s] {
			errorf(colorsFlag, "invalid sort column %q; valid: format, %v", s, scan.StatNames)
			return strictcli.Exit(ExitUsage)
		}
	}
	sortDesc := kwargs["sort_order"].(string) == "desc"

	// Validate --top before any scanning: -1 and 0 mean "all groups";
	// anything below -1 is a usage error (R8).
	top := kwargs["top"].(int)
	if top < -1 {
		errorf(colorsFlag, "invalid --top %d; must be -1 (all), 0 (all), or a positive count", top)
		return strictcli.Exit(ExitUsage)
	}

	// LOC is expensive: read files only when a LOC stat is shown or sorted
	// by. Sorting by a LOC stat forces the computation even when the stat
	// is not selected for display.
	needLOC := false
	for s := range selected {
		if scan.IsLOCStat(s) {
			needLOC = true
		}
	}
	for _, s := range sortBy {
		if scan.IsLOCStat(s) {
			needLOC = true
		}
	}

	listNoExt := kwargs["list_no_ext"].(bool)

	res, err := scan.Scan(scan.Options{
		Root:       root,
		Method:     kwargs["method"].(string),
		Formats:    kwargs["formats"].(string),
		Depth:      kwargs["depth"].(int),
		Exclude:    stringList(kwargs, "exclude"),
		Ignored:    kwargs["ignored"].(string),
		Hidden:     kwargs["hidden"].(string),
		TypeFilter: kwargs["type"].(string),
		NeedLOC:    needLOC,
		ListNoExt:  listNoExt,
		TextExts:   config.TextExtensions(),
		TextMimes:  config.TextMimetypes(),
		Aliases:    config.CanonicalFormats(),
	})
	if err != nil {
		errorf(colorsFlag, "%s", err)
		return strictcli.Exit(ExitGeneral)
	}

	// The payload is supplied in both modes (strictcli §19.4): the framework
	// decides what to do with it, so nothing here branches on --json to build
	// it. The rendering below is the human mode's, and it is the one thing
	// machine mode must not do -- stdout carries the envelope alone (§19.1).
	sortedGroups := make([]scan.Group, len(res.Groups))
	copy(sortedGroups, res.Groups)
	scan.SortGroups(sortedGroups, sortBy, sortDesc)
	ctx.Payload(jsonout.Build(configAbs, res, sortedGroups, selected, listNoExt))
	if ctx.JSON() {
		return strictcli.Exit(ExitSuccess)
	}

	// Colors apply only when requested AND stdout is a TTY; without active
	// colors the width is pinned to 80 so --no-colors output is
	// byte-identical to non-TTY output (R30).
	colorsActive := colorsFlag && term.IsTerminal(int(os.Stdout.Fd()))
	width := 80
	if colorsActive {
		if tw, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && tw > 0 {
			width = tw
		}
	}

	render.Render(os.Stdout, res, render.Options{
		Show:       kwargs["show"].(string),
		Combined:   kwargs["combined"].(bool),
		Collapse:   kwargs["singletons"].(string) == "collapse",
		SortBy:     sortBy,
		SortDesc:   sortDesc,
		Top:        top,
		ListNoExt:  listNoExt,
		Legend:     kwargs["legend"].(bool),
		Colors:     colorsActive,
		Human:      kwargs["human"].(bool),
		Style:      kwargs["style"].(string),
		TypeFilter: kwargs["type"].(string),
		Stats:      statsOrdered,
		Width:      width,
		Theme:      config.LoadTheme(render.DetectThemeName()),
		Config:     configAbs,
	})
	return strictcli.Exit(ExitSuccess)
}
