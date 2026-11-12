# Markvault

Distraction-free, local-first desktop notes for a folder of Markdown files.  
An open-source, customizable alternative to Obsidian.

## Features

- **Markdown files are canonical** — works directly on your `.md` files; no proprietary vault format.
- **Local-first** — your notes stay on disk; SQLite is used only as a rebuildable index/cache outside the vault.
- **Editor + preview** — CodeMirror 6 editing with a Markdown preview toggle (`react-markdown` + `remark-gfm`, raw HTML disabled).
- **Fast search** — SQLite FTS5 full-text search with ranked results and snippets.
- **Conflict-safe saves** — debounced autosave, atomic writes, line-ending preservation, and explicit handling for external edits.
- **Basic file operations** — create, rename, and move notes to OS trash (no silent permanent delete in MVP).

## Tech stack

- **Desktop:** [Wails v2](https://wails.io/) (Go backend + web frontend)
- **Frontend:** TypeScript, React, Vite
- **Storage/index:** local filesystem + SQLite (`modernc.org/sqlite`, FTS5)

## Development

Prerequisites: Go, Node/npm, Wails v2 CLI, and Linux WebView/GTK dependencies.

```bash
# Run checks
go test ./...
cd frontend && npm run build

# Browser end-to-end tests (starts Vite automatically)
cd frontend && npx playwright install chromium  # once per machine
cd frontend && npm run test:e2e

# Run the desktop app (requires a GUI/WebView environment)
wails dev
```

## Build

```bash
# Standard build
wails build

# Ubuntu 26.04 / webkit2gtk-4.1, if Wails reports missing webkit2gtk-4.0
wails build -m -skipbindings -tags webkit2_41
```

## Status

MVP in progress. Core vault open/read/write/search/conflict flows are implemented; GUI smoke testing, watcher/index edge cases, CRUD ergonomics, and broader test coverage are ongoing.

## License

[MIT](./LICENSE)
