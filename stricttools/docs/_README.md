+++
title = "README.md"
+++
# dirstat

Fast, single-binary directory statistics CLI: every file under a tree grouped by format, with counts, sizes, and lines of code, as a colored terminal table or as JSON.

It is built for developers and agents sizing up an unfamiliar repository, and for tooling that wants the same numbers as a machine-readable document. Every classification list and format alias table is compiled into one static binary, so a scan spawns no subprocesses, reads no configuration file it was not explicitly handed, and classifies files across a worker pool.

## Quick start

```
go install github.com/smm-h/dirstat@latest
```

Summarize the current directory:

```
dirstat scan
```

Summarize a project, machine output for tooling:

```
dirstat scan ~/src/myproject --json
```

## Example

```
$ dirstat scan .

Summary
──────────────────────────────────────────────────
  Directories: 31
  Files: 60
  Files without extension: 17
  Max depth: 5
  Symlinks (skipped): 0
  Executables: 14
  Unique formats: 11
  Files sniffed: 17
  Unreadable (skipped): 0

Files by Format
──────────────────────────────────────────────────
┌────────┬───────┬────────────┬───────────┬─────────┐
│ Format │ Count │ Total Size │ Total LOC │ ...     │
├────────┼───────┼────────────┼───────────┼─────────┤
│ go     │ 21    │ 79.6KB     │ 2,893     │ ...     │
│ md     │ 1     │ 13.1KB     │ 232       │ ...     │
└────────┴───────┴────────────┴───────────┴─────────┘
```

## Commands

:-: table-commands

Every flag is documented in `dirstat scan --help` and in the generated [.stricttools/docs/cli-scan.md](.stricttools/docs/cli-scan.md).

## Grouping methods

| Method | Behavior |
|--------|----------|
| `ext` | Group by file extension only; text/binary decided by an embedded extension list; no content sniffing |
| `type` | Content-sniff every file; group by detected MIME type |
| `hybrid` (default) | Files with an extension behave as in `ext`; extensionless files are sniffed and grouped by MIME type |

## Format names

`--formats` decides how the group name the method chose is spelled. It changes no
verdict: the same files are read, sniffed and classified either way.

| Mode | Behavior |
|--------|----------|
| `raw` (default) | The extension or sniffed MIME type verbatim, so `a.mjs` and `b.js` are separate groups and an extensionless script counts under `text/plain` |
| `canonical` | Alias formats merge (`mjs`/`cjs` into `js`, `h` into `c`, `hpp` into `cpp`, `text/x-python` into `py`), and an extensionless file with a shebang is named by its interpreter (`#!/usr/bin/env -S uv run python` counts under `py`) |

The alias table is embedded from `internal/config/data/canonical_formats.txt`. The
shebang reader strips an `env` wrapper (including its `-S` form and `NAME=value`
assignments), resolves the `uv run X` and `uvx X` runner forms, and drops a
trailing version (`python3.12` reads as `python`); an interpreter it does not
know falls through to content sniffing.

## Configuration

`dirstat scan --config <path>` reads scan defaults from a TOML file. There is no default config path and no auto-discovery — dirstat never picks up config from XDG, HOME, the current directory, or the scanned tree; the file is used only when `--config` is passed.

Allowed keys are exactly the scan-semantic flags (names with underscores): `exclude`, `method`, `formats`, `depth`, `ignored`, `hidden`, `type`, `stats`, `sort_by`, `sort_order`. Rendering and output options (`colors`, `style`, `output`, `top`, ...) cannot be set from a file. Values are validated up front with the same rules as the flags; any unknown key, wrong type, or invalid value is a hard error before scanning starts.

A key may come from the file or from the command line, never both: setting a key in the file and also passing its flag is a hard error, so there is no silent override. Flags for keys the file does not set remain usable alongside `--config`.

```toml
# scan.toml
exclude = ["node_modules", ".git", "dist"]  # replaces the built-in default list
method = "ext"
formats = "canonical"
depth = 3
stats = ["count", "total-size"]
```

By default `--exclude` uses a curated built-in list (`.git`, `node_modules`, `.venv`, `vendor`, `build`, `dist`, `target`, and other common cache/IDE directories — see `dirstat scan --help`). Any explicit `--exclude` or a config-file `exclude` key replaces the list entirely; `exclude = []` scans everything.

## Machine output

`--json` is strictcli's machine mode: stdout carries exactly one document, the framework's envelope, and the scan document is its `payload` member. No ANSI codes, raw integer values, and no table. The payload's shape is a consumer contract — field names are stable, evolution is additive-only, and the command declares the shape as a JSON Schema the framework validates before writing it.

```json
{
  "interface_version": 1,
  "app": "dirstat",
  "app_version": "0.1.0",
  "command": "scan",
  "exit_code": 0,
  "payload": {
    "root": "/abs/path",
    "method": "hybrid",
    "summary": {
      "directories": 3, "files": 6, "files_without_extension": 1,
      "max_depth": 2, "symlinks": 0, "executables": 0,
      "unique_formats": 5, "files_sniffed": 1, "unreadable": 0
    },
    "groups": [
      {
        "format": "go", "text": true, "count": 2,
        "total_size": 39, "min_size": 10, "max_size": 29, "avg_size": 20,
        "total_loc": 4, "min_loc": 1, "max_loc": 3, "avg_loc": 2
      }
    ],
    "no_extension_files": ["Makefile"]
  },
  "dry_run": false,
  "preview": [],
  "preview_error": null,
  "diagnostics": []
}
```

LOC fields are `null` for binary groups. Stats absent from `--stats` are omitted. `no_extension_files` is present only with `--list-no-ext`. The binary's version is the envelope's `app_version` and is not repeated inside the payload.

## Behavior notes

- Rendering-only flags (`--show`, `--combined`, `--singletons`, `--legend`, `--colors`, `--human`, `--style`, `--top`) have no effect on the machine payload.
- Without active colors (piped output or `--no-colors`), the table width is pinned to 80 columns so the output is byte-identical to non-TTY output.
- Symlinks are never followed, but every symlink encountered in a traversed directory is counted.
- Unreadable directories and files are skipped and counted under `Unreadable (skipped)` — never a warning spew, never a crash.
- Gitignore matching is fully in-process (no `git` subprocess): patterns come from every `.gitignore` in the work tree, `.git/info/exclude`, and the global `core.excludesFile`. Outside a git work tree, `--ignored exclude`/`only` behave as if no patterns exist.
- LOC counting is skipped entirely (no file reads) unless a LOC stat is selected or sorted by.
- The text-extension and text-mimetype lists and both color themes are embedded in the binary; there are no runtime config files.
