+++
title = "dirstat v1 specification"
description = "Authoritative v1 specification for dirstat, with numbered requirements for the CLI, traversal, classification, canonical naming, statistics, and output."
+++
# dirstat — v1 specification

dirstat summarizes the files in a directory tree, grouped by format, with aggregate
statistics (counts, sizes, lines of code) rendered as a colored terminal table or as
JSON. It is a clean-slate Go redesign of an internal Python prototype, built for
speed: no subprocesses, parallel scanning, single static binary.

This document is the authoritative spec for the initial implementation. Every
requirement is numbered (R1, R2, ...) so audits can address them individually.

## 1. Project shape

- R1. Go module `github.com/smm-h/dirstat`, Go directive `go 1.25.7`, flat
  `package main` at the repo root (no `cmd/`), matching safegit/saferm layout.
- R2. CLI built on `github.com/smm-h/strictcli/go/strictcli`, unpinned latest.
  `.strictcli/schema.json` committed.
- R3. rlsbl-managed: `.rlsbl/config.json` with `targets: ["go"]`, pipeline
  `{"type": "go", "local": true, "install_paths": ["."]}`, `private: false`,
  pre-release hooks running `go vet ./...`, `go build -o /dev/null .`, and
  `go test ./... -race -count=1`. JSONL changelog in use from the first commit.
- R4. selfdoc-managed root templates only: `selfdoc.json` with
  `root_files: [".stricttools/docs/_README.md", ".stricttools/docs/_CLAUDE.md"]`; no docs site, no deploy
  config. Generated `README.md`/`CLAUDE.md` are committed and chmod 444.
- R5. Version `0.1.0` in `VERSION`; binary version via `-X main.Version` ldflags
  with `debug.ReadBuildInfo()` fallback. The symbol name must match the one
  `.goreleaser.yml` injects: the linker silently ignores an `-X` naming a
  symbol that does not exist. MIT LICENSE,
  `Copyright (c) 2026 smm-h`.
- R6. Dependencies: strictcli, `github.com/gabriel-vasile/mimetype` (content
  sniffing), go-git's `plumbing/format/gitignore` package (ignore matching),
  `golang.org/x/term` (terminal size), `github.com/mattn/go-runewidth`
  (display-cell width for table column measurement, R28).
  `github.com/smm-h/go-toml-edit` was added with §11 (scan config).
  No CGO (`CGO_ENABLED=0` must work).

## 2. CLI surface

- R7. Single command: `dirstat scan [where]`. `where` is an optional positional
  argument, default `"."`, the directory to summarize. A missing or non-directory
  path is a hard error (exit code for usage/input error, message to stderr).
- R8. Flags (strictcli conventions: every flag and the positional argument
  declares exactly one presence -- `Required()`, `Optional()` or `Default(v)`;
  a closed set is declared as `Choices(Ch(<value>, <help>) ...)`, one record per
  value, so `--help` describes each value beneath its flag; auto `--no-*`
  negation on bools; short names only where listed). `scan` is `read_only`, so
  a declared default is legal on it -- the mutating-default ban reaches only
  commands that write. The Default column below reads `optional` where the flag
  declares no value at all:

  | Flag | Type | Default | Semantics |
  |---|---|---|---|
  | `--method` | choice: `ext`, `type`, `hybrid` | `hybrid` | Grouping method (§3) |
  | `--formats` | choice: `raw`, `canonical` | `raw` | How the chosen group's name is spelled (R48, R49) |
  | `--depth` | int | `-1` | Max directory depth below root; `-1` = unlimited; root is depth 0 |
  | `--exclude` | string, repeatable, unique | curated list (R45) | Exact base-name matches to skip (dirs and files) |
  | `--config` | string | `""` (none) | Path to a TOML scan-config file (§12); no default path, never auto-discovered |
  | `--ignored` | choice: `include`, `exclude`, `only` | `exclude` | Gitignored-path handling (§5) |
  | `--hidden` | choice: `include`, `exclude` | `include` | Dot-prefixed entries; the root dir itself is never treated as hidden |
  | `--stats` | string, repeatable, unique | optional (absent = all) | Which stats to compute/show; valid: `count`, `total-size`, `min-size`, `max-size`, `avg-size`, `total-loc`, `min-loc`, `max-loc`, `avg-loc`; invalid value = hard error |
  | `--type` | choice: `text`, `binary`, `both` | `both` | Filter groups by text/binary classification |
  | `--sort-by` | string, repeatable, unique | `count` | Sort columns, in precedence order; valid: `format` plus the nine stat names; invalid = hard error |
  | `--sort-order` | choice: `asc`, `desc` | `desc` | Applied to all sort columns |
  | `--top` | int | `-1` | Keep only the first N groups after sorting (table only); `-1` or `0` = all; values below `-1` are a usage error; in split mode applies per table |
  | `--show` | choice: `summary`, `table`, `both` | `both` | Which sections to render (table output only) |
  | `--combined` | bool | `true` | One merged table vs separate text/binary tables |
  | `--singletons` | choice: `show`, `collapse` | `show` | Collapse one-file formats into a `(singletons)` row (table only, §7) |
  | `--list-no-ext` | bool | `false` | List the paths of extensionless files after the summary |
  | `--legend` | bool | `true` | Text/binary color legend under the combined table |
  | `--colors` | bool | `true` | ANSI colors (auto-disabled when stdout is not a TTY) |
  | `--human` | bool | `true` | Human-readable sizes and thousands separators (table only) |
  | `--style` | choice: `unicode`, `ascii` | `unicode` | Table border character set |

- R9. Flag semantics that the prototype got wrong and dirstat must define cleanly:
  rendering-only flags (`--show`, `--combined`, `--singletons`, `--legend`,
  `--colors`, `--human`, `--style`, `--top`) have no effect on the machine
  payload. `--sort-by`/`--sort-order` order groups in both outputs. `--stats`
  limits which stats are computed and emitted in both outputs. `--list-no-ext`
  adds data to both outputs (a section in table mode, a field in the payload).

## 3. Grouping methods

- R10. Extension normalization: last-dot suffix, lowercased, dot stripped
  (`Path.suffix` semantics: `archive.tar.gz` → `gz`; dotfiles like `.bashrc` and
  bare names like `Makefile` have no extension).
- R11. `ext`: group name is the extension, or `(no extension)`; text/binary decided
  by the extension list only; no content sniffing ever.
- R12. `type`: every file is content-sniffed; group name is the detected MIME type
  or `(unknown)` when detection fails; text/binary via the MIME rule (R15).
- R13. `hybrid` (default): a file whose extension is in the text-extensions list is
  text without sniffing (the map is trusted on a hit). A file whose extension
  misses the list is content-sniffed: the group name stays the extension, and
  the MIME rule (R15) alone decides text/binary — a missing map entry never
  means binary. Extensionless files are sniffed and grouped by MIME type,
  falling back to `(no extension)` when sniffing yields nothing. A sniff that
  cannot read the file makes the file unreadable (R20).

Added after 0.4.0, and numbered from the end of the requirement list per the
numbering convention: `--formats` decides how the group name chosen by R11–R13
is spelled, without changing which files are grouped together by method,
which files are sniffed, or the text/binary verdict.

- R48. `--formats raw` (default) counts every file under the group name R11–R13
  produced, verbatim: the normalized extension, the sniffed MIME type, or
  `(no extension)`/`(unknown)`. `--formats canonical` maps that name through an
  embedded alias table (`internal/config/data/canonical_formats.txt`, `old new`
  pairs, one per line, `#` comments) before the file is counted, so alias
  formats merge into one group: `mjs`/`cjs` into `js`, `mts`/`cts` into `ts`,
  `h` into `c`, `hh`/`hpp` into `cpp`, and the script MIME types the sniffer
  emits (`text/x-python`, `text/x-shellscript`, `text/x-perl`, `text/x-ruby`,
  `text/x-php`, `text/x-lua`, `text/x-tcl`) into their format names. A name the
  table does not list is unchanged. A malformed line in the table is a hard
  error when the table is read, never a skipped line. `formats` is also a scan
  config key (R42), with the flag/file conflict rule of R43 applying to it.
- R49. Under `--formats canonical`, an extensionless file whose first line is a
  shebang is named by its interpreter, before the MIME sniff gets a say: the
  sniffer has no signature for most scripts and answers `text/plain`, which
  names nothing. An `env` wrapper is stripped (its options, its `-S` and
  `--split-string` forms, and any `NAME=value` assignments), the `uv run X` and
  `uvx X` runner forms resolve to `X`, and a trailing version suffix is dropped
  (`python3.12` reads as `python`). The mapping is `python*` → `py`,
  `bash`/`sh`/`zsh`/`dash`/`ksh` → `sh`, `node`/`deno`/`bun` → `js`, `perl` →
  `pl`, `ruby` → `rb`, `php` → `php`, `fish` → `fish`, `awk`/`gawk` → `awk`,
  `Rscript` → `r`, `lua` → `lua`. An interpreter outside the mapping falls
  through to the sniff, whose result still goes through the alias table (R48).
  The sniff still runs either way: it decides text/binary (R15) and counts
  toward `Files sniffed`, and the shebang only names the group. `ext`, which
  reads no content at all (R11), is untouched.
- R50. The payload shape is unchanged by `--formats` (R34): only group names,
  their counts and `unique_formats` differ, so `raw` output stays byte-identical
  to what pre-`--formats` versions produced. Tests: table-driven unit tests for
  the alias-table parser and the shebang parser (env forms, runner forms,
  version stripping, unknown-interpreter fallthrough), a config-key test, and
  integration tests pinning the merged groups and a shebang-named group against
  the real embedded table.

## 4. Text/binary classification

- R14. Extension-classified files are text iff the extension is in the embedded
  text-extensions list. A zero-byte file is text under every method, whatever
  its extension: an empty file has no bytes that could make it binary, and it
  contributes 0 LOC. Where the extension decides the verdict, the size
  short-circuits it and no content is read; where the method groups by MIME
  type (R12, and R13's extensionless path) the sniff still runs to name the
  group but cannot override emptiness.
- R15. MIME-classified files are text iff the MIME type starts with `text/` or is in
  the embedded text-mimetypes list. The result is a strict bool.
- R16. Both lists are embedded via `go:embed`, seeded from the prototype's
  lists and extended since (about 210 extensions incl. `svg`, Godot's text
  formats, `mod`/`sum` and the shader languages, but deliberately not `lock`,
  which is text for some tools and binary for others and must therefore be
  sniffed per file; about 45 mimetypes incl. `image/svg+xml`,
  `application/json`, `application/x-empty`, `inode/x-empty`). No runtime config
  files, no lookup in cwd/HOME/XDG. Changing the lists means a rebuild.

## 5. Traversal

- R17. Walk with `filepath.WalkDir`-level efficiency (DirEntry, no redundant stat).
  Root is depth 0. Directories beyond `--depth`, matching `--exclude`, hidden
  (when excluded), or gitignored (when excluded) are pruned: not counted, not
  descended into.
- R18. Gitignore handling is fully in-process using go-git's gitignore package:
  patterns from every `.gitignore` in the tree, `.git/info/exclude`, and the
  global `core.excludesFile`. Modes: `include` = no matching at all; `exclude` =
  drop ignored paths; `only` = keep only ignored paths (`only` prunes ignored
  dirs' contents from exclusion — files under an ignored dir count as ignored).
  When the target is not inside a git work tree, `exclude` and `only` behave as
  if no patterns exist; this must be documented in the flag help.
- R19. Symlinks are never followed — neither file nor directory symlinks — but every
  symlink encountered in any traversed directory is counted, so the symlink count
  does not depend on traversal order (fixes a prototype bug where symlinks under
  pruned dirs were missed; symlinks under pruned dirs are legitimately out of
  scope, but all symlinks in traversed dirs must be counted even when they would
  be filtered by name rules).
- R20. Unreadable directories and files (permission errors, stat failures) are
  skipped and counted in a summary stat `Unreadable (skipped)`. Never a warning
  spew; never a crash.
- R21. Per-file work (LOC counting, content sniffing) runs in a worker pool sized to
  `runtime.GOMAXPROCS(0)`. Results must be deterministic regardless of
  scheduling.

## 6. Statistics

- R22. Per group: `count`, `total-size`, `min-size`, `max-size`, `avg-size`, and for
  text groups `total-loc`, `min-loc`, `max-loc`, `avg-loc`. Binary groups have no
  LOC values (rendered `-`, JSON `null`). Averages are rounded to nearest
  integer, not floored. Stats not requested via `--stats` are neither computed
  nor shown (LOC is expensive; skipping it must actually skip the file reads),
  with one exception: sorting by a LOC key via `--sort-by` forces LOC
  computation even when LOC stats are not displayed.
- R23. LOC = number of `\n` bytes, plus 1 if the file is non-empty and does not end
  with `\n`. Counted with buffered `bytes.Count`-style reads (no line decoding,
  no size cap). Read errors count the file as unreadable (R20), not LOC 0.
- R24. Summary stats: `Directories`, `Files`, `Files without extension`,
  `Max depth`, `Symlinks (skipped)`, `Executables`, `Unique formats`,
  `Files sniffed`, `Unreadable (skipped)`. `Files without extension` counts only
  files that passed all filters including `--type` (fixes prototype bug).
  `Executables` = regular files with any execute permission bit. `Files sniffed`
  = number of content-sniff operations performed.

## 7. Table output

- R25. Section order: summary (heading `Summary`, a rule line, `label: value`
  lines), then table(s), then optional extensionless-file list, respecting
  `--show`.
- R26. Combined mode: one table `Files by Format` with text rows and binary rows
  distinguished by color. Split mode (`--no-combined`): `Text Files by Format`
  and `Binary Files by Format`; the binary table omits LOC columns. Row-building
  logic must be shared, not duplicated per table (prototype wart).
- R27. Columns in fixed order: `Format`, `Count`, `Total Size`, `Min Size`,
  `Max Size`, `Avg Size`, `Total LOC`, `Min LOC`, `Max LOC`, `Avg LOC` — each
  present only if its stat is selected. Sizes humanized in 1024-based units with
  one decimal (except bare bytes) when `--human`; raw integers otherwise. Counts
  get thousands separators when `--human`.
- R28. Width adaptation: query terminal width (`x/term`; fallback 80 when not a
  TTY). If the table overflows, shrink only the Format column down to a floor of
  10, truncating cells with a `..` marker. Column widths are measured in
  terminal display cells (runewidth), not bytes, and truncation never splits a
  UTF-8 rune. MIME-type group names (any name containing a `/`) truncate from
  the *left*, keeping the subtype and prefixing `..`: their prefix is shared
  and uninformative, so tail truncation would render every `application/*`
  group as the same indistinguishable `applicat..`. All other names keep their
  prefix and take the marker as a suffix. Whenever colors are inactive (non-TTY or `--no-colors`), width is
  pinned to 80 so that `--no-colors` output is byte-identical to non-TTY output
  (R30).
- R29. Border styles: `unicode` (light box-drawing, the prototype's look) and
  `ascii` (`+ - |`). One shared renderer parameterized by charset.
- R30. Colors: ANSI 256-color themes (dark and light) embedded as data; theme keys:
  text/binary/header fg+bg, stat label, stat value, error, border. Dark unless
  `COLORFGBG` clearly indicates a light background. Colors apply only when
  `--colors` is true AND stdout is a TTY. `--no-colors` output must be
  byte-identical to non-TTY output. The theme's `error` color is applied to the
  `error:` prefix of stderr error messages when stderr is a TTY and `--colors`
  is true (stderr coloring is independent of machine mode).
- R31. Legend (`■ Text files ■ Binary files`) renders only when `--legend`,
  `--combined`, `--type both`, and colors are active (it is meaningless
  without color).
- R32. `--singletons collapse`: groups with exactly one file are merged into a
  single `(singletons)` pseudo-row (count = number of merged groups, sizes and
  text-only LOC aggregated); the row participates in sorting by its aggregate
  values and renders in a neutral (header) color. In split mode the collapse
  happens within each table independently.
- R33. Empty result → `No files found.` instead of an empty table.

## 8. Machine output

- R34. Machine output is strictcli's `--json` mode, and dirstat declares no
  output-format flag of its own. Under `--json`, stdout carries exactly one
  document -- the framework's envelope -- and no table; the scan document is the
  envelope's `payload` member. No ANSI codes ever, raw integer values (no
  humanization). The payload is a consumer contract (external tools will parse
  it): field names are stable, evolution is additive-only.
- R35. Payload shape, declared to the framework as a JSON Schema on the scan
  command and validated at emission, so a deviating document fails the run
  instead of reaching a consumer:

  ```json
  {
    "root": "/abs/path",
    "method": "hybrid",
    "summary": {
      "directories": 0, "files": 0, "files_without_extension": 0,
      "max_depth": 0, "symlinks": 0, "executables": 0,
      "unique_formats": 0, "files_sniffed": 0, "unreadable": 0
    },
    "groups": [
      {
        "format": "go", "text": true, "count": 0,
        "total_size": 0, "min_size": 0, "max_size": 0, "avg_size": 0,
        "total_loc": 0, "min_loc": 0, "max_loc": 0, "avg_loc": 0
      }
    ],
    "no_extension_files": ["relative/path"]
  }
  ```

  The binary's version is the envelope's `app_version` and is not restated in
  the payload. Groups are ordered per `--sort-by`/`--sort-order`. Stats absent
  from `--stats` are omitted from group objects; LOC fields are `null` for
  binary groups. `no_extension_files` (paths relative to root, sorted) present
  only with `--list-no-ext`. `--top` and `--singletons` do not affect the
  payload (R9).

## 9. Sorting

- R36. Multi-key sort over `--sort-by` columns with a single `--sort-order`
  direction. `format` compares case-insensitively. For LOC keys, binary groups
  sort as 0. Ties broken by format name ascending so output is deterministic.

## 10. Testing

- R37. Unit tests colocated per package, table-driven. Integration tests in
  `internal/test/` build the real binary once (TestMain pattern from saferm) and
  run it against fixture trees created with `t.TempDir()` (including: nested
  gitignore, hidden files, symlinks, permission-denied entries where the
  platform allows, extensionless files, empty dirs).
- R38. Golden-file tests for machine output: fixture tree in, committed golden
  envelope compared byte-for-byte (after normalizing the absolute root path and
  the app version, and re-indenting the one-line document).
  Golden files live in `internal/test/testdata/`.
- R39. Determinism test: two runs over the same fixture tree produce identical
  output despite the parallel worker pool.
- R40. `go vet ./...` clean, `gofmt` clean, `go test ./... -race` passes.

## 11. Scan config file

Added after 0.1.0. A TOML file supplying scan defaults, loaded and fully
validated before scanning begins. The file is used only when explicitly requested
via `--config <path>` — dirstat never auto-discovers config from XDG, HOME, the
current directory, or the scanned tree. Only scan-semantic keys are allowed;
rendering and output options are rejected with a hard error naming the key.

- R41. `--config <path>`: when set, the file MUST exist and parse as TOML; a
  missing, unreadable, or malformed file is a hard error (exit 2) reporting the
  path and, for parse errors, line/column (go-toml-edit `ParseError`). Loaded and
  fully validated before any scanning starts.
- R42. Allowed keys (flag names with underscores), exactly the scan-semantic set:
  `exclude` (string array), `method`, `formats`, `depth` (integer), `ignored`, `hidden`,
  `type`, `stats` (string array), `sort_by` (string array), `sort_order`. Values
  are validated with the same rules as the corresponding flags (choices, valid
  stat names, uniqueness). Any unknown key, rendering/output key (e.g. `colors`,
  `style`, `output`, `top`), wrong type, or invalid value is a hard error (exit
  2) naming the key and the allowed key set. `where` cannot be set from config.
- R43. Conflict rule (no silent override): if a key is present in the config file
  AND its flag was explicitly passed on the command line, that is a hard error
  (exit 2) naming the key. Explicit passing is detected from raw `os.Args`
  (matching `--flag value` and `--flag=value` forms for the R42 flag set).
  CLI flags for keys the file does not set remain usable alongside `--config`.
- R44. Effective settings: registered flag defaults, overlaid by config-file
  values, overlaid by CLI values only for keys absent from the file (R43 makes
  the file/CLI overlap impossible). Rendering flags are unaffected by config.
- R45. Built-in default excludes: `--exclude` registers a curated default list:
  `.git`, `node_modules`, `.venv`, `vendor`, `build`, `dist`, `target`,
  `zig-out`, `zig-pkg`, `.next`, `.svelte-kit`, `__pycache__`, `.mypy_cache`,
  `.ruff_cache`, `.pytest_cache`, `.hypothesis`, `.gradle`, `.idea`,
  `.wrangler`, `.vscode`. The list is visible in `--help`. An explicit
  `exclude = []` in the config file (or any CLI `--exclude`) replaces it
  entirely; there is no additive merging anywhere.
- R46. Visibility: when a config file is in effect, the table summary gains a
  `Config: <path>` line (first summary line, absolute path), and the machine
  payload gains an additive optional field `"config"` (absolute path) after
  `"method"`.
  Absent entirely when `--config` is not used (additive schema evolution, R34).
- R47. Tests: unit tests for the TOML loader/validator and the argv explicitness
  scanner; integration tests covering happy path, missing file, malformed TOML,
  unknown key, rendering key, wrong type, file/CLI conflict, `exclude = []`,
  and a golden JSON fixture exercising the `config` field. The default-exclude
  behavior change is covered by updated integration expectations (e.g. `.git`
  no longer scanned by default; `--exclude ""` style overrides still honored).

## 12. Non-goals for v1

- No result caching (measure first; the subprocess elimination and parallel scan
  are expected to make it unnecessary).
- No interactive/TUI mode.
- No symlink following.
- No auto-discovered config and no runtime classification/theme data: extension
  lists, mimetype lists, and themes are embedded-only, and dirstat never picks up
  config from XDG, HOME, cwd, or the scanned tree. (The explicit `--config` scan
  file of §11 is the sole, opt-in file input.)
- Never bump to 1.0.0 (stays 0.x until explicitly authorized).
