+++
title = "Embedded Data"
description = "How dirstat classifies files as text or binary, how canonical format aliases merge group names, and which directories are excluded by default."
+++
# Embedded Data

dirstat embeds several data files into the binary at build time via `go:embed`. These control file classification and default traversal behavior. There are no runtime config files for classification -- changing the lists requires a rebuild.

The embedded data lives under `internal/config/data/` and is loaded by the `internal/config` package.

## Text vs. binary classification

Every file dirstat encounters is classified as either **text** or **binary**. This classification determines whether lines of code (LOC) are counted for the file (binary files have no LOC stats), how the file's row is colored in table output (text and binary rows use different theme colors), and whether the file appears in `--type text` or `--type binary` filtered output. The classification is strict: every file is one or the other, never "unknown." The mechanism depends on the grouping method:

| Method | Files with extension | Files without extension |
| --- | --- | --- |
| `ext` | Extension list lookup | Always binary |
| `type` | Content sniffing | Content sniffing |
| `hybrid` (default) | Extension list lookup, falling back to content sniffing when the extension is not in the list | Content sniffing |

A zero-byte file is text under every method: an empty file has no bytes that could make it binary, and it contributes 0 lines of code.

## Text extension list

The text extension list is embedded from `internal/config/data/text_extensions.txt` via `go:embed` and contains approximately 210 extensions covering programming languages, shell scripts, web technologies, data formats, documentation markup, build systems, game engines, and shaders. A file is text if its normalized extension (last-dot suffix, lowercased, dot stripped) appears in the list.

A miss is not a verdict. In `ext` mode the file is binary, but in `hybrid` mode the file is content-sniffed and the MIME rule decides, so text formats outside the list still count their lines of code. Deliberately ambiguous extensions are left out on purpose: `.lock` is TOML text for uv and Poetry but binary for other tools, so every `.lock` file is judged by its own content.

:-: table-text-extensions

## Text MIME type list

The text MIME type list is embedded from `internal/config/data/text_mimetypes.txt` via `go:embed` and enumerates non-`text/` MIME types that should be treated as text for classification purposes. When content sniffing is performed (always in `type` mode; for extensionless files in `hybrid` mode), dirstat uses the `github.com/gabriel-vasile/mimetype` library to detect the file's MIME type. The file is classified as text if:

1. The detected MIME type starts with `text/` (e.g., `text/plain`, `text/html`), **or**
2. The detected MIME type is in the text MIME type list

The second rule exists because many text-based formats have MIME types under `application/` or other top-level types rather than `text/`. For example, `application/json` is clearly text but does not start with `text/`.

The list contains 48 MIME types:

| MIME Type | Category |
| --- | --- |
| `application/json` | Data interchange |
| `application/xml` | Markup |
| `application/javascript` | Programming |
| `application/ecmascript` | Programming |
| `application/x-javascript` | Programming (legacy) |
| `application/x-sh` | Shell |
| `application/x-shellscript` | Shell |
| `application/x-perl` | Programming |
| `application/x-python` | Programming |
| `application/x-ruby` | Programming |
| `application/x-php` | Programming |
| `application/x-httpd-php` | Programming |
| `application/x-awk` | Shell |
| `application/x-gawk` | Shell |
| `application/x-nawk` | Shell |
| `application/x-sed` | Shell |
| `application/sql` | Data |
| `application/graphql` | Data |
| `application/ld+json` | Data interchange |
| `application/manifest+json` | Data interchange |
| `application/x-yaml` | Data |
| `application/yaml` | Data |
| `application/toml` | Data |
| `application/x-toml` | Data |
| `application/x-wine-extension-ini` | Config |
| `application/xhtml+xml` | Markup |
| `application/rss+xml` | Markup |
| `application/atom+xml` | Markup |
| `application/soap+xml` | Markup |
| `application/xslt+xml` | Markup |
| `application/mathml+xml` | Markup |
| `application/x-tex` | Documentation |
| `application/x-latex` | Documentation |
| `application/rtf` | Documentation |
| `application/postscript` | Documentation |
| `application/x-troff` | Documentation |
| `application/x-troff-man` | Documentation |
| `application/x-troff-me` | Documentation |
| `application/x-troff-ms` | Documentation |
| `application/x-info` | Documentation |
| `application/x-texinfo` | Documentation |
| `application/x-maker` | Documentation |
| `application/csv` | Data |
| `application/x-empty` | Empty file |
| `inode/x-empty` | Empty file |
| `image/svg+xml` | Vector graphics (text-based) |

## Canonical format aliases

The alias table is embedded from `internal/config/data/canonical_formats.txt` via `go:embed` and is read only under `--formats canonical`. Each line is an `old new` pair: a raw group name -- a normalized extension or a sniffed MIME type -- and the format name that group is counted under. Lines starting with `#` are comments, and a line that is not a pair is a hard error rather than a skipped line.

The table merges module variants into their language (`mjs` and `cjs` into `js`, `mts` and `cts` into `ts`), headers into their language (`h` into `c`, `hh` and `hpp` into `cpp`), and the script MIME types the sniffer emits into format names (`text/x-python` into `py`, `text/x-shellscript` into `sh`, and the same for Perl, Ruby, PHP, Lua and Tcl). A group name the table does not list is counted under its raw name.

Canonical mode also names extensionless scripts by their shebang interpreter, before the MIME sniff gets a say -- the sniffer has no signature for most scripts and answers `text/plain`, which names nothing. The interpreter is read after stripping an `env` wrapper (its options, its `-S`/`--split-string` forms, and any `NAME=value` assignments), unwrapping the `uv run X` and `uvx X` runner forms, and dropping a trailing version suffix, so `#!/usr/bin/env -S uv run python` and `#!/usr/bin/python3.12` both count under `py`. The sniff still runs: it decides text vs. binary, and the shebang only names the group. An interpreter outside the mapping falls through to the sniffed MIME type, which then goes through the alias table.

## Default excludes

When `--exclude` is not explicitly passed (and no config file sets `exclude`), dirstat skips these directories and files by exact base-name match during traversal. Excluded directories are pruned entirely -- their contents are not descended into, counted, or classified.

:-: table-default-excludes

## Overriding exclusions

The default exclude list is replaced entirely when you provide your own exclusion list, whether via the `--exclude` command-line flag or the `exclude` key in a TOML config file. There is no additive merging -- any explicit exclusion list completely supersedes all built-in defaults, so you must re-specify any default entries you still want excluded.

### Via the `--exclude` flag

Pass `--exclude` one or more times on the command line to set the exclusion list for a single invocation. Each `--exclude` value is an exact base-name match against directories and files encountered during traversal. When any `--exclude` flag is present, the entire built-in default list is replaced by the values you specify, so only those names are skipped.

```
dirstat scan --exclude node_modules --exclude .git
```

This scans everything except `node_modules` and `.git`. The 18 other built-in defaults are no longer excluded.

To scan with no exclusions at all, the config file approach is required (see below).

### Via a config file

Set the `exclude` key in a TOML config file passed with `--config` to control exclusions persistently across invocations. The config file uses a string array where each element is an exact base-name to skip during traversal. Setting `exclude` to an empty array scans everything with no exclusions at all.

```toml
# scan.toml
exclude = ["node_modules", ".git", "dist"]
```

```
dirstat scan --config scan.toml
```

To scan everything with no exclusions:

```toml
exclude = []
```

A key set both in the config file and on the command line is a hard error -- use one or the other. See the [spec](spec.md) (R41-R47) for full config file semantics.
