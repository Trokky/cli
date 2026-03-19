# Trokky CLI - Implementation Plan

Porting the Go CLI to match the TypeScript reference at `packages/trokky`.

---

## 1. Configuration System — DONE

### 1.1 Switch from JSON to YAML config
- [x] Add `gopkg.in/yaml.v3` dependency
- [x] Change config file from `~/.trokky/config.json` to `~/.trokky/config.yaml`
- [x] Update `internal/config/config.go` to read/write YAML

### 1.2 Align config data model with TypeScript
- [x] Add `version` field (`"1.0"`)
- [x] Rename `activeInstance` to `default` (instance name, not URL)
- [x] Switch from `map[url]InstanceConfig` to `map[name]InstanceConfig`
- [x] Add fields: `url`, `token`, `refreshToken`, `authType`, `tokenExpiresAt`, `description`, `addedAt`, `updatedAt`

### 1.3 Config manager functions
- [x] `Load`/`Save`, `AddInstance`, `RemoveInstance`, `ListInstances`, `GetInstance`, `GetDefaultInstance`, `SetDefaultInstance`, `ConfigExists`, `MaskToken`

### 1.4 Credential resolution (3-tier priority)
- [x] CLI flags > env vars > config file
- [x] `ResolveCredentials`, `RequireCredentials` with helpful error messages

### 1.5 `config` command group
- [x] `config add/remove/list/use/path`

---

## 2. Authentication — DONE

- [x] OAuth2 Device Flow (`trokky login <url>`)
- [x] Token refresh (`IsTokenExpired`, `RefreshAccessToken`, `GetValidToken`)
- [x] Auto-refresh in `FromContext` before API calls
- [x] `logout` command as shortcut for `config remove`

---

## 3. HTTP Client — DONE

- [x] `Get`/`Post`/`Put`/`Delete` convenience methods
- [x] Auto-extract `.data` from `{success: true, data: ...}` envelope
- [x] `SetToken`, 30s HTTP timeout
- [x] `FromContext` with credential resolution + OAuth2 auto-refresh
- [x] `UploadFile` (multipart form upload)

---

## 4. Backup System (v2.0) — DONE

- [x] Zip-based output with `manifest.json`, per-document files, media streaming
- [x] Schema analyzer: dependency graph, topological sort, compatibility validation
- [x] `--output`, `--collections`, `--skip-media`, `--description` flags
- [x] Defer close for resource safety, path traversal protection

---

## 5. Restore System (v2.0) — DONE

- [x] Zip-based input, manifest validation
- [x] Media upload first (builds old→new ID mappings)
- [x] Document restore in dependency order with reference rewriting
- [x] Document sanitization (nulls, empty objects, old media format)
- [x] Singleton handling (PUT upsert with fallback, checks both target and backup schema)
- [x] `--input`, `--collections`, `--with-dependencies`, `--clean`, `--overwrite`, `--dry-run`

---

## 6. Documents CRUD — DONE

- [x] `documents list` with `--limit`, `--offset`, `--filter`, `--sort`/`--order`, `--status`, `--expand`, `--format`, `--count`
- [x] `documents get` with `--expand`, `--field` (dot-path extraction)
- [x] `documents create` from file, `--data`, or stdin
- [x] `documents update` from file, `--data`, or stdin
- [x] `documents delete` with multiple IDs, `--force`
- [x] `docs` alias

---

## 7. Utility Commands — DONE

- [x] `clean` with `--collections`, `--media-only`/`--documents-only`, `--dry-run`, `--confirm`
- [x] `status` (uses new credential resolution)
- [x] `export` / `import` kept as convenience aliases
- [x] `generate-types` kept as Go-specific extra

---

## 8. Output & UX — DONE

- [x] JSON pretty-print, IDs-only format, dot-path field extraction
- [x] Stdin support for piping
- [x] Global `--quiet`/`-q` flag (suppresses instance info)
- [x] `confirmPrompt` shared helper

---

## 9. Refactor — DONE

- [x] All commands use new config/client/auth system
- [x] Shared helpers: `ExtractDocID`, `ParseSchemas`, `ParseDocuments`, `splitCSV`, `confirmPrompt`
- [x] Auth type constants (`AuthTypeAPIToken`, `AuthTypeOAuth2`)
- [x] `NormalizeBaseURL` shared URL helper
- [x] `ManifestVersion` constant

---

## 10. Build & Release — DONE

- [x] `go.mod` updated with `gopkg.in/yaml.v3`
- [x] Browser opening uses stdlib `os/exec`
- [x] `.goreleaser.yml` unchanged (still valid)
- [ ] Verify cross-platform builds — manual step

---

## Progress Tracker

| Area | Status | Tests |
|------|--------|-------|
| 1. Config System | Done | 45 |
| 2. Authentication | Done | 24 |
| 3. HTTP Client | Done | 33 |
| 4. Backup v2.0 | Done | 22 |
| 5. Restore v2.0 | Done | 16 |
| 6. Documents CRUD | Done | — |
| 7. Utility Commands | Done | — |
| 8. Output & UX | Done | — |
| 9. Refactor | Done | — |
| 10. Build & Release | Done | — |
| **Total** | **Done** | **140** |

---

## Remaining / Future Work

These items are deferred for a separate effort:

### Features not yet ported from TypeScript
- [ ] `trokky create <project-name>` — interactive project scaffolding (templates, adapters, mail, auth, studio, i18n)
- [ ] `trokky migrate` — content migration from WordPress, Strapi, Contentful, Sanity, JSON, CSV

### Polish / nice-to-have
- [ ] Spinner/progress feedback (e.g., `github.com/briandowns/spinner`)
- [ ] Table output format for `documents list`
- [ ] `--status <published|draft>` flag on `documents create/update`
- [ ] Client-side schema validation before create/update
- [ ] `Authenticate(credentials)` method on client (username/password API login)
- [ ] Silent mode (`--silent` — suppress ALL output except data)
- [ ] Verify cross-platform builds (Linux, macOS, Windows; amd64, arm64)
