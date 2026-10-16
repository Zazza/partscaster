# CLAUDE.md

The rules for this repository are in [AGENTS.md](AGENTS.md) — read it in full before making changes. Digest:

- **`make lint && make test` must be green** before handing off. CI checks `main` and pull requests only, not `develop`.
- **Keep API clients at parity:** worker endpoint → `yue.Service` → `App` binding → `api.js` → MCP tool, in one change.
  Edits to slots/groups/presets or `worker/fx_blocks.json` → `make mcp-data` and commit every regenerated file.
- **Docs in two languages:** `docs/ru/` and `docs/en/` (same file names), `README.md` + `README.ru.md`. Change both in the
  same commit. Tests read `docs/{ru,en}/effects.md` and `docs/ru/ui-guide.md` — keep their anchors.
- **Releases:** CHANGELOG `Unreleased` → version with date, plus a new first entry in `frontend/src/releaseNotes.js`
  (plain words, ru + en, version = tag) in the same commit. A human tags; CI builds and publishes.
- **Architecture:** DI via `Player` / `yue.Service`, worker HTTP only through `Client.call*`; screens in
  `src/components/`, state in `src/composables/`; UI strings via i18n (ru + en).
- **Tests follow the spec**, not current behaviour. **No personal data, no secrets.**
- Conventional Commits; comments in Russian, identifiers in English. Deliberate decisions (no worker auth,
  SIGSTOP pause, external player) are not to be "fixed" without asking.
