+++
title = "CLAUDE.md"
+++
# dirstat

:-: var key="project.description"

The authoritative specification is `.stricttools/docs/spec.md` (numbered requirements R1–R47).

## Project structure

Root files:

- `main.go` — app setup, registers the scan command via strictcli
- `scan.go` — scan command registration, flag validation, orchestration
- `exitcodes.go` — exit codes: 0 success, 1 general, 2 usage
- `version.go` — version from ldflags or debug.ReadBuildInfo

Internal packages (one bullet per package, from the package doc comments in source):

:-: list-modules path="internal"

## Build and test

```bash
CGO_ENABLED=0 go build .        # build (must work without CGO)
go test ./... -race -count=1    # full suite
go vet ./... && gofmt -l .      # lint gate
go test ./internal/test -run TestGolden -args -update   # regenerate golden JSON
```

## Key conventions

- **CLI framework:** strictcli (`github.com/smm-h/strictcli/go/strictcli`). Handlers receive `map[string]interface{}` and return the exit code; never call `os.Exit` in handlers; errors go to stderr with an `error:` prefix.
- **Machine output is the framework's `--json`:** stdout carries the strictcli envelope alone, and the scan document is its `payload` member. dirstat declares no output-format flag of its own, and `internal/jsonout` owns both the document and the JSON Schema the command declares for it.
- **The payload shape is a consumer contract:** field names are stable, evolution is additive-only. Never remove or rename fields.
- **Rendering-only flags never affect the payload** (`--show`, `--combined`, `--singletons`, `--legend`, `--colors`, `--human`, `--style`, `--top`).
- **Determinism:** worker-pool results are stored by index; groups are sorted with a full tie-break. Two runs must produce identical output.
- **Embedded classification data:** the text lists, the canonical format alias table (`--formats canonical`) and the themes are `go:embed` data under `internal/config/data/`; changing them means a rebuild. The only runtime file input is the explicit `--config` TOML scan-config (spec §11): never auto-discovered, scan-semantic keys only, file/CLI overlap is a hard error.
- **`.strictcli/schema.json`** is committed; regenerate with `./dirstat --dump-schema` after changing the CLI surface.
- **Releases:** rlsbl-managed (`.rlsbl/`), JSONL changelog required for every commit.
