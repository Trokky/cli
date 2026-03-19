# Trokky CLI - Implementation Plan

Porting the Go CLI to match the TypeScript reference at `packages/trokky`.

---

## 1. Configuration System — DONE

- [x] YAML config at `~/.trokky/config.yaml` with named instances
- [x] 3-tier credential resolution (CLI flags > env vars > config file)
- [x] Config manager: Load/Save/Add/Remove/List/Get/SetDefault/MaskToken
- [x] `config add/remove/list/use/path` subcommands
- [x] `NormalizeBaseURL`, `RequireCredentials` helpers

## 2. Authentication — DONE

- [x] OAuth2 Device Flow (`trokky login <url>`)
- [x] Token refresh (IsTokenExpired, RefreshAccessToken, GetValidToken)
- [x] Auto-refresh in `FromContext` before API calls
- [x] `logout` command (shortcut for `config remove`)

## 3. HTTP Client — DONE

- [x] Get/Post/Put/Delete methods with auto-extract envelope
- [x] UploadFile (multipart form)
- [x] 30s HTTP timeout, SetToken
- [x] `FromContext` with credential resolution + OAuth2 auto-refresh + `--quiet`

## 4. Backup System (v2.0) — DONE

- [x] Zip-based output with manifest v2.0, per-document files, media streaming
- [x] Schema analyzer: dependency graph, topological sort, compatibility validation
- [x] Resource safety (defer close), path traversal protection

## 5. Restore System (v2.0) — DONE

- [x] Zip-based input, manifest validation
- [x] Media upload first (builds old→new ID mappings)
- [x] Document restore in dependency order with reference rewriting
- [x] Singleton handling (PUT upsert with fallback, checks target + backup schema)
- [x] Document sanitization, `--clean/--overwrite/--dry-run/--with-dependencies`

## 6. Documents CRUD — DONE

- [x] `documents list` with limit/offset/filter/sort/status/expand/format(json/table/ids-only)/count
- [x] `documents get` with expand, dot-path field extraction
- [x] `documents create/update` from file/--data/stdin, `--status`, `--validate`
- [x] `documents delete` with multiple IDs, `--force`
- [x] `docs` alias

## 7. Utility Commands — DONE

- [x] `clean` with `--collections/--media-only/--documents-only/--dry-run/--confirm`
- [x] `create` — interactive project scaffolding (templates, adapters, mail, auth, studio, captcha, i18n)
- [x] `status`, `export`, `import`, `generate-types` (all use new client)

## 8. Output & UX — DONE

- [x] JSON pretty-print, table output, IDs-only format
- [x] Dot-path field extraction, stdin support
- [x] Global `--quiet/-q` flag
- [x] Spinner for OAuth2 login polling
- [x] `confirmPrompt` shared helper
- [x] Client-side schema validation (`--validate` flag)

## 9. Refactor — DONE

- [x] All commands use new config/client/auth system
- [x] Shared helpers: ExtractDocID, ParseSchemas, ParseDocuments, splitCSV, confirmPrompt
- [x] Constants: AuthTypeAPIToken/OAuth2, ManifestVersion, NormalizeBaseURL
- [x] Flag validation on `trokky create`

## 10. Build & Release — DONE

- [x] Dependencies: yaml.v3, briandowns/spinner
- [x] Cross-platform verified: linux/darwin/windows on amd64/arm64
- [x] .goreleaser.yml unchanged (still valid)

---

## Stats

| Metric | Count |
|--------|-------|
| Go source files | 25 |
| Test files | 7 |
| Tests | 151 |
| Commits | 11 |
| Lines added | ~6,500 |

---

## Remaining / Future Work

| Item | Effort | Description |
|------|--------|-------------|
| `trokky migrate` | Large | Content migration from WordPress, Strapi, Contentful, Sanity, JSON, CSV |
| Batch API operations | Medium | Concurrent document import/delete with worker pools |
| Pagination | Medium | Loop until no more results for `?limit=10000` endpoints |
| `ExportMedia` streaming | Small | Stream to disk instead of buffering in memory (only affects legacy `export` cmd) |
| Table output sorting | Small | Sort table rows by a column |
| Shell completion | Small | Generate bash/zsh/fish completion scripts (cobra built-in) |
