"""Selfdoc directive: render the defaultExcludes list from scan.go as a table."""

import os
import re


# Brief descriptions for each excluded directory.
_DESCRIPTIONS = {
    ".git": "Git repository metadata",
    "node_modules": "npm dependencies",
    ".venv": "Python virtual environment",
    "vendor": "Vendored dependencies (Go, PHP, etc.)",
    "build": "Build output",
    "dist": "Distribution/build output",
    "target": "Build output (Rust, Maven, etc.)",
    "zig-out": "Zig build output",
    "zig-pkg": "Zig package cache",
    ".next": "Next.js build output",
    ".svelte-kit": "SvelteKit build output",
    "__pycache__": "Python bytecode cache",
    ".mypy_cache": "mypy type-checker cache",
    ".ruff_cache": "Ruff linter cache",
    ".pytest_cache": "pytest cache",
    ".hypothesis": "Hypothesis test data",
    ".gradle": "Gradle build cache",
    ".idea": "JetBrains IDE config",
    ".wrangler": "Cloudflare Wrangler state",
    ".vscode": "VS Code config",
}


def _project_root():
    """Return the project root: the nearest ancestor holding selfdoc.json.

    Found by marker rather than by a parent count, so this resolves correctly
    wherever the docs tree sits inside the repository.
    """
    directory = os.path.dirname(os.path.abspath(__file__))
    while True:
        if os.path.isfile(os.path.join(directory, "selfdoc.json")):
            return directory
        parent = os.path.dirname(directory)
        if parent == directory:
            raise RuntimeError("no selfdoc.json above " + __file__)
        directory = parent


def resolve(attrs, config, body):
    project_root = _project_root()
    scan_go = os.path.join(project_root, "scan.go")

    with open(scan_go) as f:
        src = f.read()

    # Extract the slice literal contents.
    m = re.search(r"defaultExcludes\s*=\s*\[]interface\{\}\{([^}]+)\}", src)
    if not m:
        return "*Could not parse defaultExcludes from scan.go.*"

    raw = m.group(1)
    entries = re.findall(r'"([^"]+)"', raw)

    lines = ["| Directory | Description |", "| --- | --- |"]
    for entry in entries:
        desc = _DESCRIPTIONS.get(entry, "")
        lines.append(f"| `{entry}` | {desc} |")

    return "\n".join(lines)
