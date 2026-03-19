# Trokky CLI - Implementation Plan

Porting the Go CLI to match the TypeScript reference at `packages/trokky`.

---

## 1. Configuration System

### 1.1 Switch from JSON to YAML config
- [x] Add `gopkg.in/yaml.v3` dependency
- [x] Change config file from `~/.trokky/config.json` to `~/.trokky/config.yaml`
- [x] Update `internal/config/config.go` to read/write YAML

### 1.2 Align config data model with TypeScript
- [x] Add `version` field (`"1.0"`)
- [x] Rename `activeInstance` to `default` (instance name, not URL)
- [x] Switch from `map[url]InstanceConfig` to `map[name]InstanceConfig`
- [x] Add fields to `InstanceConfig`:
  - [x] `url` (base URL of the API)
  - [x] `token`
  - [x] `refreshToken` (optional, for OAuth2)
  - [x] `authType` (`api-token` | `oauth2`)
  - [x] `tokenExpiresAt` (ISO string, optional)
  - [x] `description` (optional label)
  - [x] `addedAt` (ISO timestamp)
  - [x] `updatedAt` (ISO timestamp)

### 1.3 Config manager functions
- [x] `loadConfig()` / `saveConfig()`
- [x] `addInstance(name, instance, setAsDefault)`
- [x] `removeInstance(name)` (reassign default if removed)
- [x] `listInstances()` -> `{instances, defaultInstance}`
- [x] `getInstance(name)`
- [x] `getDefaultInstance()` -> `{name, instance}`
- [x] `setDefaultInstance(name)`
- [x] `configExists()`
- [x] `maskToken(token)` (show first 4 and last 4 chars)

### 1.4 Credential resolution (3-tier priority)
- [x] Priority 1: CLI flags (`--url` + `--token`)
- [x] Priority 2: Environment variables (`TROKKY_URL`, `TROKKY_TOKEN`, `TROKKY_INSTANCE`)
- [x] Priority 3: Config file (default instance or `--instance` flag)
- [x] `resolveCredentials(options)` function
- [x] `requireCredentials(options)` (exit with helpful error if missing)
- [x] Helpful error messages showing all 3 methods when no credentials found

### 1.5 `config` command group
- [x] `trokky config add <name>` (interactive prompts if `--url`/`--token` not provided)
- [x] `trokky config remove <name>` (with `--force` to skip confirmation)
- [x] `trokky config list` / `trokky config ls`
- [x] `trokky config use <name>` (set default instance)
- [x] `trokky config path` (show config file location)

---

## 2. Authentication

### 2.1 Remove username/password login
- [x] Remove current `cmd/login.go` (username/password flow)
- [x] Remove `internal/auth/auth.go` (credential prompting)

### 2.2 OAuth2 Device Flow (`trokky login <url>`)
- [x] POST to `{baseUrl}/auth/device` with `client_id` and `scope`
- [x] Display user code and verification URL
- [x] Auto-open browser via `open` (or equivalent Go package)
- [x] Poll `{baseUrl}/auth/token` with device code
- [x] Handle polling responses: `authorization_pending`, `slow_down`, `access_denied`, `expired_token`
- [x] Save token + refresh token to config on success
- [x] `--name` flag to set instance name (default: derived from hostname)
- [x] `--set-default` flag (default: true)
- [ ] Spinner/progress feedback during polling (using simple print for now)

### 2.3 Token refresh
- [x] `isTokenExpired(tokenExpiresAt, bufferSeconds)` (default 5min buffer)
- [x] `refreshAccessToken(instanceName, instance)` via POST to `{url}/auth/token` with `grant_type: refresh_token`
- [x] `getValidToken(instanceName, instance)` - returns current or refreshed token
- [ ] Auto-refresh in `createCliClient()` before API calls (deferred to Section 3)
- [x] Helpful error when refresh fails and re-login is needed

### 2.4 Remove `logout` command
- [x] Replace with `trokky config remove <name>`

---

## 3. HTTP Client

### 3.1 Align client with TypeScript `HttpClient`
- [x] Config struct: `baseUrl`, `apiToken`, `timeout`
- [x] Auth tokens struct: `accessToken`, `refreshToken`, `expiresAt`
- [x] Generic `request(method, path, data)` method
- [x] Auto-extract `.data` from `{success: true, data: ...}` response envelope
- [x] Error extraction from `{success: false, error: {message: ...}}`
- [x] Methods: `Get`, `Post`, `Put`, `Delete`
- [ ] `Authenticate(credentials)` method (deferred — not needed until documents CRUD)
- [x] `SetToken(token)` method
- [x] Request timeout (default 30s)

### 3.2 CLI client helper
- [x] `FromContext(cmd)` function that:
  - [x] Resolves credentials (3-tier)
  - [x] Auto-refreshes OAuth2 tokens
  - [x] Displays instance info to stderr
  - [x] Returns client
- [x] Common credential flags on all commands (`--url`, `--token`, `--instance`)

---

## 4. Backup System (v2.0 format)

### 4.1 Switch to zip-based backup
- [x] Output a single `.zip` file instead of a flat directory
- [x] Use `archive/zip` from Go stdlib
- [x] `--output <file>` required flag (e.g., `backup.zip`)

### 4.2 Manifest (`manifest.json`)
- [x] `version: "2.0"`
- [x] `timestamp` (ISO string)
- [x] `source: {url, description}`
- [x] `schemas[]` (full schema definitions)
- [x] `dependencyGraph` (collection -> dependencies)
- [x] `restoreOrder` (topologically sorted)
- [x] `mediaIndex` (old media ID -> `{filename, mimeType, size, metadata}`)
- [x] `statistics: {totalDocuments, totalMedia, collections, backupSizeBytes}`

### 4.3 Backup structure inside zip
- [x] `manifest.json`
- [x] `collections/<name>/<docId>.json` (one file per document)
- [x] `media/<filename>` (binary media files)

### 4.4 Backup command options
- [x] `--output <file>` (required)
- [x] `--collections <csv>` (filter specific collections)
- [x] `--skip-media`
- [x] `--description <text>`
- [ ] Spinner/progress feedback (using simple print for now)

### 4.5 Schema analyzer
- [x] `buildDependencyGraph(schemas)` - scan fields for references
- [x] `getRestoreOrder(graph)` - topological sort
- [x] `validateSchemaCompatibility(source, target)` - check source schemas vs target instance
- [x] Detect reference fields (`type: "reference"`, `to: "..."`)
- [x] Detect media asset fields (`type: "media"/"image"/"video"/"audio"/"file"`)
- [x] Handle array fields (`type: "array"`, recurse into `of`)
- [x] Handle object fields (`type: "object"`, recurse into `fields`)

---

## 5. Restore System (v2.0 format)

### 5.1 Switch to zip-based restore
- [x] Read from `.zip` file
- [x] Read manifest directly from zip (no temp dir needed)
- [x] `--input <file>` required flag

### 5.2 Restore flow
- [x] Read `manifest.json` from zip
- [x] Validate manifest version (`2.0`)
- [x] Determine collections to restore (all or `--collections`)
- [x] `--with-dependencies` flag (auto-include dependencies of selected collections)
- [x] Pre-flight schema validation against target instance
- [ ] Restore media first (build old ID -> new ID mappings) — media upload deferred
- [x] Restore documents in dependency order
- [x] Reference rewriting using ID mappings
- [x] Document sanitization (null removal, empty object removal, old media format)

### 5.3 Reference scanner
- [x] `UpdateReferences(document, schema, idMappings)` - rewrite old IDs to new IDs
- [x] Recursive scanning through nested objects, arrays, and media assets
- [x] `SanitizeDocument` - removes nulls, empty objects, old media format
- [x] `StripSystemFields` - removes _id, _createdAt, etc.

### 5.4 Restore command options
- [x] `--input <file>` (required)
- [x] `--collections <csv>` (filter)
- [x] `--with-dependencies`
- [x] `--clean` (delete existing content before restore)
- [x] `--overwrite` (overwrite existing documents on conflict)
- [x] `--dry-run` (preview without applying)
- [ ] Singleton handling (use PUT/upsert, preserve IDs) — deferred
- [x] Summary with counts (documents, media, references updated)

---

## 6. Documents CRUD Commands

### 6.1 `trokky documents list <collection>`
- [x] `--limit <n>` (default: 20)
- [x] `--offset <n>`
- [x] `--filter <json>` (JSON filter conditions)
- [x] `--sort <field>` / `--order <asc|desc>`
- [x] `--status <published|draft>`
- [x] `--expand <fields>` (expand references)
- [x] `--format <json|ids-only>`
- [x] `--count` (just show count)
- [ ] `--quiet` (suppress instance info) — deferred to Section 8

### 6.2 `trokky documents get <collection> <id>`
- [x] `--expand <fields>`
- [x] `--format <json>`
- [x] `--field <path>` (extract specific field using dot-path)

### 6.3 `trokky documents create <collection> [file]`
- [x] Read from file argument, `--data` flag, or stdin
- [x] `--data <json>` (inline JSON)
- [ ] `--status <published|draft>` — deferred
- [ ] Schema validation before sending — deferred

### 6.4 `trokky documents update <collection> <id> [file]`
- [x] Read from file, `--data`, or stdin
- [x] `--merge` flag (partial update / patch)
- [ ] `--status <published|draft>` — deferred
- [ ] Schema validation — deferred

### 6.5 `trokky documents delete <collection> <id> [...ids]`
- [x] Multiple ID support
- [x] `--force` (skip confirmation)
- [x] `--quiet`

### 6.6 Alias
- [x] `trokky docs` as alias for `trokky documents`

---

## 7. Utility Commands

### 7.1 `trokky clean`
- [x] Delete all content from an instance
- [x] `--collections <csv>` (filter)
- [x] `--media-only` / `--documents-only`
- [x] `--dry-run`
- [x] `--confirm` (required for actual deletion)
- [x] Summary with counts
- [x] Failed deletion tracking and reporting

### 7.2 `trokky create <project-name>` (scaffolding)
- [ ] Deferred — large feature, separate effort

### 7.3 `trokky migrate` (content migration)
- [ ] Deferred — large feature, separate effort

### 7.4 `trokky status`
- [x] Keep existing status command
- [x] Already uses `client.FromContext` with new credential resolution

---

## 8. Output & UX Utilities

### 8.1 Output formatting
- [x] JSON output (pretty-printed) — in documents list/get/create/update
- [ ] Table output (for `documents list`) — deferred
- [x] IDs-only output (for piping) — `--format ids-only`
- [ ] Quiet mode (suppress non-essential output) — deferred
- [ ] Silent mode (suppress all output except data) — deferred

### 8.2 Dot-path field extraction
- [x] `getByDotPath(obj, "nested.field.path")` — in documents.go
- [x] Used by `documents get --field`

### 8.3 Stdin support
- [x] Detect stdin data via `os.Stdin.Stat()`
- [x] Read and parse JSON from stdin
- [x] Enable piping: `cat data.json | trokky documents create posts`

### 8.4 Schema validation (client-side)
- [ ] Deferred — would need schema fetching + validation logic

---

## 9. Remove / Refactor Existing Code

- [x] Rewrite `cmd/login.go` (OAuth2 device flow)
- [x] Remove `cmd/logout.go` (replaced by `config remove`)
- [x] Rewrite `internal/auth/auth.go` (OAuth2 + token refresh)
- [x] Keep `cmd/export.go` as convenience alias (uses new client)
- [x] Keep `cmd/import.go` as convenience alias (uses new client)
- [x] Keep `cmd/generate_types.go` as Go-specific extra (uses new client)
- [x] `cmd/status.go` already uses `client.FromContext` with new credential resolution
- [x] Rewrite `cmd/backup.go` for v2.0 zip format
- [x] Rewrite `cmd/restore.go` for v2.0 zip format
- [x] `cmd/root.go` updated with new flags and shared helpers

---

## 10. Build & Release

- [x] `go.mod` updated with `gopkg.in/yaml.v3`
- [x] Browser opening uses stdlib `os/exec` (no external dependency)
- [x] `.goreleaser.yml` unchanged (still valid)
- [ ] Verify cross-platform builds (Linux, macOS, Windows; amd64, arm64) — manual step

---

## Progress Tracker

| Area | Status | Notes |
|------|--------|-------|
| 1. Config System | Done | All sections complete (1.1-1.5) |
| 2. Authentication | Done | OAuth2 device flow, token refresh, logout removed |
| 3. HTTP Client | Done | Get/Post/Put/Delete, auto-extract envelope, token refresh, timeout |
| 4. Backup v2.0 | Done | Zip format, manifest v2.0, schema analyzer, 22 tests |
| 5. Restore v2.0 | Done | Zip-based, ref rewriting, sanitize, clean/overwrite/dry-run |
| 6. Documents CRUD | Done | list/get/create/update/delete + docs alias |
| 7. Utility Commands | Done | clean command; create/migrate deferred |
| 8. Output & UX | Done | JSON, ids-only, dot-path, stdin; table/quiet deferred |
| 9. Refactor | Done | All commands use new config/client/auth system |
| 10. Build & Release | Done | yaml.v3 added; goreleaser config unchanged |

## Pre-work completed

- [x] Test suite for existing `internal/config` (13 tests)
- [x] Test suite for existing `internal/auth` (10 tests)
- [x] Test suite for existing `internal/client` (26 tests)
- [x] Fixed `go vet` error in `internal/auth/auth.go` (non-constant format string)
- [x] Code review: fixed `t.Fatalf` in httptest handler goroutines, dead code removal, header ordering
