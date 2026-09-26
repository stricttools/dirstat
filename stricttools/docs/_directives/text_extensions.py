"""Selfdoc directive: render text_extensions.txt as a categorized table."""

import os


# Map each extension to (language/format name, category).
_EXT_INFO = {
    # Programming languages
    "py": ("Python", "Programming"),
    "rb": ("Ruby", "Programming"),
    "js": ("JavaScript", "Programming"),
    "ts": ("TypeScript", "Programming"),
    "jsx": ("JSX", "Programming"),
    "tsx": ("TSX", "Programming"),
    "mjs": ("ES module JS", "Programming"),
    "cjs": ("CommonJS", "Programming"),
    "coffee": ("CoffeeScript", "Programming"),
    "java": ("Java", "Programming"),
    "kt": ("Kotlin", "Programming"),
    "kts": ("Kotlin Script", "Programming"),
    "scala": ("Scala", "Programming"),
    "groovy": ("Groovy", "Programming"),
    "clj": ("Clojure", "Programming"),
    "cljs": ("ClojureScript", "Programming"),
    "c": ("C", "Programming"),
    "h": ("C/C++ header", "Programming"),
    "cpp": ("C++", "Programming"),
    "hpp": ("C++ header", "Programming"),
    "cc": ("C++", "Programming"),
    "cxx": ("C++", "Programming"),
    "hxx": ("C++ header", "Programming"),
    "c++": ("C++", "Programming"),
    "h++": ("C++ header", "Programming"),
    "cs": ("C#", "Programming"),
    "fs": ("F#", "Programming"),
    "vb": ("Visual Basic", "Programming"),
    "go": ("Go", "Programming"),
    "rs": ("Rust", "Programming"),
    "swift": ("Swift", "Programming"),
    "php": ("PHP", "Programming"),
    "pl": ("Perl", "Programming"),
    "pm": ("Perl module", "Programming"),
    "perl": ("Perl", "Programming"),
    "lua": ("Lua", "Programming"),
    "tcl": ("Tcl", "Programming"),
    "r": ("R", "Programming"),
    "R": ("R", "Programming"),
    "rmd": ("R Markdown", "Programming"),
    "m": ("Objective-C", "Programming"),
    "mm": ("Objective-C++", "Programming"),
    "asm": ("Assembly", "Programming"),
    "s": ("Assembly", "Programming"),
    "S": ("Assembly", "Programming"),
    "d": ("D", "Programming"),
    "nim": ("Nim", "Programming"),
    "zig": ("Zig", "Programming"),
    "v": ("V", "Programming"),
    "odin": ("Odin", "Programming"),
    "elm": ("Elm", "Programming"),
    "hs": ("Haskell", "Programming"),
    "lhs": ("Literate Haskell", "Programming"),
    "ml": ("OCaml/SML", "Programming"),
    "mli": ("OCaml interface", "Programming"),
    "ocaml": ("OCaml", "Programming"),
    "erl": ("Erlang", "Programming"),
    "hrl": ("Erlang header", "Programming"),
    "ex": ("Elixir", "Programming"),
    "exs": ("Elixir script", "Programming"),
    "lisp": ("Lisp", "Programming"),
    "cl": ("Common Lisp", "Programming"),
    "lsp": ("Lisp", "Programming"),
    "scm": ("Scheme", "Programming"),
    "ss": ("Scheme", "Programming"),
    "rkt": ("Racket", "Programming"),
    "f": ("Fortran", "Programming"),
    "f90": ("Fortran 90", "Programming"),
    "f95": ("Fortran 95", "Programming"),
    "for": ("Fortran", "Programming"),
    "ftn": ("Fortran", "Programming"),
    "pas": ("Pascal", "Programming"),
    "pp": ("Pascal", "Programming"),
    "inc": ("Include file", "Programming"),
    "ada": ("Ada", "Programming"),
    "adb": ("Ada body", "Programming"),
    "ads": ("Ada spec", "Programming"),
    "cob": ("COBOL", "Programming"),
    "cbl": ("COBOL", "Programming"),
    "cpy": ("COBOL copybook", "Programming"),
    "pro": ("Prolog", "Programming"),
    "P": ("Prolog", "Programming"),
    # Game engines and shaders
    "gd": ("GDScript", "Game/Shader"),
    "gdshader": ("Godot shader", "Game/Shader"),
    "tscn": ("Godot scene", "Game/Shader"),
    "tres": ("Godot resource", "Game/Shader"),
    "godot": ("Godot project", "Game/Shader"),
    "uid": ("Godot resource UID", "Game/Shader"),
    "glsl": ("GLSL shader", "Game/Shader"),
    "hlsl": ("HLSL shader", "Game/Shader"),
    "wgsl": ("WGSL shader", "Game/Shader"),
    "frag": ("Fragment shader", "Game/Shader"),
    "vert": ("Vertex shader", "Game/Shader"),
    # Shell/scripting
    "awk": ("AWK", "Shell"),
    "sed": ("sed", "Shell"),
    "ps1": ("PowerShell", "Shell"),
    "psm1": ("PowerShell module", "Shell"),
    "psd1": ("PowerShell data", "Shell"),
    "bat": ("Batch", "Shell"),
    "cmd": ("Windows command", "Shell"),
    "sh": ("Shell", "Shell"),
    "bash": ("Bash", "Shell"),
    "zsh": ("Zsh", "Shell"),
    "fish": ("Fish", "Shell"),
    "ksh": ("Korn shell", "Shell"),
    "csh": ("C shell", "Shell"),
    "tcsh": ("TENEX C shell", "Shell"),
    "vim": ("Vim script", "Shell"),
    "vimrc": ("Vim config", "Shell"),
    "el": ("Emacs Lisp", "Shell"),
    "elisp": ("Emacs Lisp", "Shell"),
    # Web
    "html": ("HTML", "Web"),
    "htm": ("HTML", "Web"),
    "xhtml": ("XHTML", "Web"),
    "shtml": ("Server-side HTML", "Web"),
    "css": ("CSS", "Web"),
    "scss": ("SCSS", "Web"),
    "sass": ("Sass", "Web"),
    "less": ("Less", "Web"),
    "stylus": ("Stylus", "Web"),
    "svg": ("SVG", "Web"),
    "vue": ("Vue", "Web"),
    "svelte": ("Svelte", "Web"),
    "astro": ("Astro", "Web"),
    "svx": ("Svelte Markdown", "Web"),
    # Data/Config
    "xml": ("XML", "Data/Config"),
    "xsl": ("XSLT", "Data/Config"),
    "xslt": ("XSLT", "Data/Config"),
    "xsd": ("XML Schema", "Data/Config"),
    "dtd": ("DTD", "Data/Config"),
    "json": ("JSON", "Data/Config"),
    "json5": ("JSON5", "Data/Config"),
    "jsonl": ("JSON Lines", "Data/Config"),
    "jsonc": ("JSON with comments", "Data/Config"),
    "yaml": ("YAML", "Data/Config"),
    "yml": ("YAML", "Data/Config"),
    "toml": ("TOML", "Data/Config"),
    "ini": ("INI", "Data/Config"),
    "cfg": ("Config", "Data/Config"),
    "conf": ("Config", "Data/Config"),
    "config": ("Config", "Data/Config"),
    "env": ("Env file", "Data/Config"),
    "htaccess": ("Apache config", "Data/Config"),
    "csv": ("CSV", "Data/Config"),
    "tsv": ("TSV", "Data/Config"),
    "sql": ("SQL", "Data/Config"),
    "graphql": ("GraphQL", "Data/Config"),
    "gql": ("GraphQL", "Data/Config"),
    "proto": ("Protocol Buffers", "Data/Config"),
    "avsc": ("Avro schema", "Data/Config"),
    "prisma": ("Prisma schema", "Data/Config"),
    "plist": ("Property list", "Data/Config"),
    "hcl": ("HCL", "Data/Config"),
    "tf": ("Terraform", "Data/Config"),
    "tfvars": ("Terraform vars", "Data/Config"),
    "nix": ("Nix", "Data/Config"),
    "dhall": ("Dhall", "Data/Config"),
    "jsonnet": ("Jsonnet", "Data/Config"),
    # Documentation
    "md": ("Markdown", "Docs"),
    "markdown": ("Markdown", "Docs"),
    "mdown": ("Markdown", "Docs"),
    "mkd": ("Markdown", "Docs"),
    "mkdn": ("Markdown", "Docs"),
    "rst": ("reStructuredText", "Docs"),
    "txt": ("Plain text", "Docs"),
    "text": ("Plain text", "Docs"),
    "tex": ("TeX", "Docs"),
    "latex": ("LaTeX", "Docs"),
    "bib": ("BibTeX", "Docs"),
    "adoc": ("AsciiDoc", "Docs"),
    "asciidoc": ("AsciiDoc", "Docs"),
    "org": ("Org-mode", "Docs"),
    "pod": ("Perl POD", "Docs"),
    "rdoc": ("RDoc", "Docs"),
    "wiki": ("Wiki markup", "Docs"),
    "creole": ("Creole markup", "Docs"),
    "textile": ("Textile", "Docs"),
    # Build
    "mk": ("Makefile", "Build"),
    "makefile": ("Makefile", "Build"),
    "cmake": ("CMake", "Build"),
    "mod": ("Go module file", "Build"),
    "sum": ("Go checksum file", "Build"),
    "gradle": ("Gradle", "Build"),
    "sbt": ("SBT", "Build"),
    "cabal": ("Cabal", "Build"),
    "cargo": ("Cargo", "Build"),
    "gemfile": ("Gemfile", "Build"),
    "gemspec": ("Gemspec", "Build"),
    "podfile": ("CocoaPods", "Build"),
    "package": ("Package file", "Build"),
    "composer": ("Composer", "Build"),
    "pipfile": ("Pipfile", "Build"),
    "pyproject": ("pyproject", "Build"),
    # Git
    "gitignore": (".gitignore", "Build"),
    "gitattributes": (".gitattributes", "Build"),
    "gitmodules": (".gitmodules", "Build"),
    "gitconfig": (".gitconfig", "Build"),
    # Docker
    "dockerfile": ("Dockerfile", "Build"),
    "dockerignore": (".dockerignore", "Build"),
    # Other config
    "editorconfig": ("EditorConfig", "Build"),
    "eslintrc": ("ESLint config", "Build"),
    "prettierrc": ("Prettier config", "Build"),
    "babelrc": ("Babel config", "Build"),
    "npmrc": ("npm config", "Build"),
    "yarnrc": ("Yarn config", "Build"),
    "inputrc": ("Readline config", "Build"),
    # Misc
    "log": ("Log file", "Docs"),
    "diff": ("Diff/patch", "Docs"),
    "patch": ("Patch", "Docs"),
    "service": ("systemd unit", "Data/Config"),
    "desktop": ("Desktop entry", "Data/Config"),
}

# Category display order.
_CATEGORY_ORDER = ["Programming", "Game/Shader", "Web", "Data/Config", "Docs", "Shell", "Build"]


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
    ext_file = os.path.join(project_root, "internal", "config", "data", "text_extensions.txt")

    with open(ext_file) as f:
        content = f.read()

    # Parse all extensions from the file.
    extensions = []
    for line in content.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        extensions.extend(line.split())

    # Deduplicate while preserving order.
    seen = set()
    unique_exts = []
    for ext in extensions:
        if ext not in seen:
            seen.add(ext)
            unique_exts.append(ext)

    # Group by category.
    categories = {cat: [] for cat in _CATEGORY_ORDER}
    uncategorized = []
    for ext in unique_exts:
        info = _EXT_INFO.get(ext)
        if info:
            name, cat = info
            categories.setdefault(cat, []).append((ext, name))
        else:
            uncategorized.append((ext, ext))

    lines = []
    for cat in _CATEGORY_ORDER:
        entries = categories.get(cat, [])
        if not entries:
            continue
        lines.append(f"**{cat}**")
        lines.append("")
        lines.append("| Extension | Format |")
        lines.append("| --- | --- |")
        for ext, name in entries:
            lines.append(f"| `.{ext}` | {name} |")
        lines.append("")

    if uncategorized:
        lines.append("**Other**")
        lines.append("")
        lines.append("| Extension | Format |")
        lines.append("| --- | --- |")
        for ext, name in uncategorized:
            lines.append(f"| `.{ext}` | {name} |")
        lines.append("")

    return "\n".join(lines)
