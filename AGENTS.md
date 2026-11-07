# Markvault workspace notes

- Stack: Go + Wails v2 + React/TS (Vite) + SQLite (`modernc.org/sqlite`, FTS5). Markdown files in the selected vault are canonical; SQLite index/cache lives in XDG app data outside the vault.
- Planning baseline: `/home/ubl/.local/share/kilo/plans/1794024561722-markvault-planning.md`.
- Git remote: `origin` → `https://github.com/pallab-js/Marky_Go.git` (default pushed branch: `master`). Do not add another remote, amend published history, or force-push unless explicitly requested.


## Validation commands

```bash
go vet ./...
go test ./...
cd frontend && npm run build
# GUI-dependent: wails dev
# Linux/Ubuntu 26.04 with webkit2gtk-4.1 may need:
wails build -m -skipbindings -tags webkit2_41
```

## Safety rules

- Never treat SQLite as note truth; index must be rebuildable from vault files.
- Confine note operations to the active vault root; skip dot-segment paths and do not follow symlinks (MVP).
- Do not silently overwrite on external+local edit conflicts (Decision 8).
- Delete moves to OS trash when available; if unavailable, block with an actionable error (no permanent delete fallback in MVP).
- Preview must not enable raw HTML; neutralize dangerous URL schemes (Decision 13).
