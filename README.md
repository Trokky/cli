# Trokky CLI

Command-line tool for managing [Trokky CMS](https://github.com/Trokky/trokky) instances.

This is the only official Trokky CLI — the former TypeScript CLI shipped inside `@trokky/client` is retired. It targets Trokky v2 servers.

## Install

**macOS (Homebrew):**

```bash
brew install trokky/tap/trokky
```

**macOS / Linux (script):**

```bash
curl -sSL https://raw.githubusercontent.com/Trokky/cli/main/install.sh | sh
```

Auto-detects OS and architecture, downloads the latest release.

**Windows:**

Download `trokky_*_windows_amd64.zip` from [Releases](https://github.com/Trokky/cli/releases), extract, and add to your PATH.

**From source:**

```bash
go install github.com/Trokky/cli@latest
```

## Quick Start

```bash
# Login to your Trokky instance (opens browser for OAuth2)
trokky login https://cms.example.com

# Check connection
trokky status

# List documents
trokky docs list posts

# Create a document
trokky docs create posts --data '{"title": "Hello World"}'

# Backup everything
trokky backup --output backup.zip

# Scaffold a new project
trokky create my-site
```

## Commands

| Command | Description |
|---------|-------------|
| `login <url>` | Authenticate via OAuth2 device flow |
| `logout [name]` | Remove stored credentials |
| `status` | Show instance health and collections |
| `config add\|remove\|list\|use\|path` | Manage instance configurations |
| `documents list\|get\|create\|update\|delete` | CRUD operations on documents |
| `backup --output <file>` | Create a zip backup with manifest |
| `restore --input <file>` | Restore from a backup file |
| `clean --confirm` | Delete all content from an instance |
| `export <collection> [file]` | Export a collection to JSON |
| `import <collection> <file>` | Import documents from JSON |
| `create <project-name>` | Scaffold a new Trokky project |
| `migrate` | Upgrade a v0.1.x project to Trokky v2 packages |
| `generate-types` | Generate TypeScript types from schemas |

## Authentication

Trokky CLI supports three ways to provide credentials (in priority order). The
`--url` value points at the API mount (`/api` by default), not the site root;
`trokky login` takes the instance URL and resolves the mount for you.

**1. CLI flags:**

```bash
trokky status --url https://cms.example.com/api --token your-token
```

**2. Environment variables:**

```bash
export TROKKY_URL=https://cms.example.com/api
export TROKKY_TOKEN=your-token
trokky status
```

**3. Stored configuration (recommended):**

```bash
# OAuth2 login (interactive, opens browser)
trokky login https://cms.example.com

# Or add a token manually
trokky config add production --url https://cms.example.com/api --token your-token

# Switch between instances
trokky config use staging
trokky config list
```

Configuration is stored in `~/.trokky/config.yaml`.

## Documents

```bash
# List with filtering and pagination
trokky docs list posts --limit 10 --status published --sort _createdAt --order desc

# Pagination: --offset or 1-based --page (both combine with --limit)
trokky docs list posts --limit 10 --page 2
trokky docs list posts --limit 10 --offset 20

# Full-text search and JSON filters
trokky docs list posts --search hello
trokky docs list posts --filter '{"featured":true}'

# Count only
trokky docs list posts --count

# Different output formats
trokky docs list posts --format table
trokky docs list posts --format ids-only -q  # clean for piping

# Get a single document
trokky docs get posts post-123
trokky docs get posts post-123 --field title  # extract a specific field

# Create from file, inline JSON, or stdin
trokky docs create posts article.json
trokky docs create posts --data '{"title": "Hello"}' --status published
cat data.json | trokky docs create posts

# Update
trokky docs update posts post-123 --data '{"title": "New Title"}'

# Delete (with confirmation)
trokky docs delete posts post-123 post-456 --force
```

### `documents list` flags

`documents list` sends the v2 server's native query format. Sorting uses prefix
notation: `--sort _createdAt --order desc` is sent as `sort=-_createdAt`, and
`--order asc` (the default) as `sort=_createdAt`. A value already written
directionally — `--sort -_createdAt` or `--sort _createdAt.desc` — is passed
through unchanged.

| Flag | Description |
|------|-------------|
| `--limit <n>` | Maximum documents to return (default 20) |
| `--offset <n>` | Number of documents to skip |
| `--page <n>` | 1-based page number, used with `--limit` |
| `--search <query>` | Full-text search query |
| `--filter <json>` | JSON filter conditions |
| `--sort <field>` | Field to sort by |
| `--order asc\|desc` | Sort direction applied to `--sort` (default `asc`) |
| `--status <status>` | Filter by status (`published` or `draft`); folded into `--filter` as `_status` |
| `--expand <fields>` | Expand reference fields |
| `--format json\|table\|ids-only` | Output format (default `json`) |
| `--count` | Print the total document count only |

## Backup & Restore

Backups use a zip format with a manifest, per-document files, and media:

```bash
# Full backup
trokky backup --output backup.zip

# Backup specific collections, skip media
trokky backup --output backup.zip --collections posts,pages --skip-media

# Dry-run restore (preview without changes)
trokky restore --input backup.zip --dry-run

# Restore with options
trokky restore --input backup.zip --clean --overwrite
trokky restore --input backup.zip --collections posts --with-dependencies
```

## Project Scaffolding

```bash
# Interactive mode
trokky create my-site

# Non-interactive with template
trokky create my-site --template full --examples -y

# Customize adapters
trokky create my-site --template minimal --data postgres --media filesystem --mail resend
```

Templates: `minimal`, `full`, `api-only`

| Flag | Values |
|------|--------|
| `-t, --template` | `minimal`, `full`, `api-only` |
| `--data` | `filesystem`, `postgres` |
| `--media` | `filesystem` |
| `--mail` | `none`, `resend`, `console` |
| `--auth` | `basic`, `oauth`, `none` |
| `--studio` | `embedded`, `separate`, `none` |
| `--captcha` | `none`, `turnstile`, `recaptcha` |
| `--i18n` | `none`, `en`, `fr`, `en-fr` |
| `--examples` | Include example schemas |
| `-y, --yes` | Skip prompts, use defaults |

The generated project depends on `trokky` ^2.0.0 and `@trokky/client`, plus
`@trokky/studio` when Studio is enabled. Trokky v2 ships the server and every
adapter in the single `trokky` package: the scaffold mounts
`TrokkyExpress` from `@trokky/trokky/express` and enables adapters through side-effect
imports — `@trokky/trokky/adapters/filesystem-data` or `@trokky/trokky/adapters/postgres-data`
for data, and `@trokky/trokky/adapters/filesystem-media` for media.

## Migrating a v0.1.x project to v2

`trokky migrate` is a codemod for projects still on the old split npm packages.
It rewrites every quoted `@trokky/*` module specifier in the project's JS, TS
and Astro sources, and updates the dependency sections of every `package.json`.
It works purely on files: no instance, URL or token is needed. `node_modules`,
`dist`, `build`, `.git`, `.astro` and symlinks are never touched, and the
default is a dry run.

Recommended order:

```bash
# 1. Back up the OLD site first — the migration changes code, not content,
#    but you want a restore point before the version bump.
trokky backup --output before-migrate.zip

# 2. See what would change.
trokky migrate --dry-run

# 3. Read the report and fix the warnings (especially the singleton ones).

# 4. Apply.
trokky migrate --write

# 5. Reinstall and build, then diff and commit.
npm install && npm run build
git diff
```

| Flag | Description |
|------|-------------|
| `--path <dir>` | Project directory (default `.`) |
| `--dry-run` | Report only; this is the default, the flag just makes it explicit |
| `--write` | Actually rewrite the files |
| `-y, --yes` | Skip the confirmation prompt |
| `--force` | Write even when the git working tree is dirty |

`--dry-run` and `--write` are mutually exclusive. When `--path` is inside a git
repository with uncommitted changes (untracked files included), `--write`
refuses to run: a clean tree is what lets you `git diff` the migration and
`git checkout` your way out of it. Pass `--force` to override.

### Package mapping

| Old package | v2 |
|-------------|----|
| `@trokky/core` | `@trokky/trokky` |
| `@trokky/routes` | `@trokky/trokky` |
| `@trokky/express` | `@trokky/trokky/express` |
| `@trokky/structure` | `@trokky/trokky/structure` |
| `@trokky/types` | `@trokky/trokky/types` |
| `@trokky/i18n` | `@trokky/trokky/i18n` |
| `@trokky/mail` | `@trokky/trokky/mail` |
| `@trokky/mail-adapter-console` | `@trokky/trokky/mail/console` |
| `@trokky/mail-adapter-resend` | `@trokky/trokky/mail/resend` |
| `@trokky/mail-adapter-smtp` | `@trokky/trokky/mail/smtp` |
| `@trokky/adapter-filesystem` | `@trokky/trokky/adapters/filesystem-data` |
| `@trokky/adapter-filesystem-data` | `@trokky/trokky/adapters/filesystem-data` |
| `@trokky/adapter-filesystem-media` | `@trokky/trokky/adapters/filesystem-media` |
| `@trokky/adapter-postgres-data` | `@trokky/trokky/adapters/postgres-data` |
| `@trokky/fields` | `@trokky/studio` |
| `@trokky/client` | unchanged, bumped to `^2.0.0` |
| `@trokky/studio` | unchanged, bumped to `^2.0.0` |

Subpaths follow their package: `@trokky/core/schema` becomes
`@trokky/trokky/schema`. The longest rule wins, so
`@trokky/mail-adapter-console` never matches `@trokky/mail` first.

In each `package.json`, the old packages are removed from `dependencies`,
`devDependencies` and `peerDependencies`, and `@trokky/trokky`, `@trokky/studio`
and `@trokky/client` are pinned to `^2.0.0`. `@trokky/trokky` is added only
where an old package was actually removed — to `dependencies`, or to
`devDependencies` when that is the only place a removal happened, never to
`peerDependencies` — so a frontend-only workspace that just uses
`@trokky/client` gets the version bump and nothing else. Key order, the file's
own indentation and its trailing newline are preserved — only the dependency
lines change.

### Warnings

Some things a text codemod must not fix by itself. The report lists them with
their file and line; they never fail the command.

**`internal-import`** — the file imports a `@trokky` package's internals, e.g.
`../node_modules/@trokky/fields/src/definitions/RichTextField/format-converter.js`.
v2 does not ship `src/` paths, and the relative form is not a bare specifier,
so it is left exactly as it is. Rewrite those imports by hand against the
package's public entry points.

**`astro-env`** — the file reads `import.meta.env.TROKKY_API_URL`. Astro inlines
that value at *build* time, not at runtime, so setting `TROKKY_API_URL` only for
the server process leaves the built site pointing at whatever was in the
environment when it was built — every media request then 404s. Set the variable
for the site build too.

**`singleton-divergence`** — the structure declares an entry as
`type: 'singleton'` but the matching schema does not set `singleton: true` (or
no schema declares that name at all). This is the data-loss trap the command
exists for: `trokky restore` will POST such a document instead of doing an
id-preserving PUT, which regenerates the document id and orphans the structure
entry pointing at the old one. Add `singleton: true` to the schema **before**
running `trokky restore` against the migrated site.

**`singleton-near-miss`** — the schema sets `isSingleton: true`. That key is a
structure navigation key and is silently ignored on a schema, so the document
still behaves as a collection. Use `singleton: true`.

## Generate Types

```bash
trokky generate-types -o ./src/types/trokky
```

Fetches `GET /collections`, then `GET /schemas/<name>` for each collection, and
writes a single self-contained `index.ts` to the output directory (default
`./src/types/trokky`). Both endpoints are authenticated, so credentials are
required — via `trokky login`, a stored config, or `--url`/`--token`.

The generated file imports nothing and contains `BaseDocument` (`_id`, `_type`,
`_createdAt`, `_updatedAt`, `_version`, `_status`), `MediaFieldValue`,
`Reference<T>`, and one `<Name>Document` interface per collection — with nested
interfaces for object fields, unions for references, and `unknown` for custom
field types. It ends with `DocumentType`, `AnyDocument`, `DocumentTypeMap` and
`DocumentOf<T>`.

```ts
export interface PostDocument extends BaseDocument {
  _type: 'post'
  title: string
  slug?: string
  cover?: MediaFieldValue | null
  author?: Reference<'author'> | AuthorDocument
  tags?: string[]
}

export type DocumentType = 'post' | 'author'
export type DocumentOf<T extends DocumentType> = DocumentTypeMap[T]
```

## Global Flags

| Flag | Description |
|------|-------------|
| `--url <url>` | Trokky API mount, e.g. `https://cms.example.com/api` |
| `--token <token>` | API token |
| `--instance <name>` | Use a specific configured instance |
| `-q, --quiet` | Suppress informational output |
| `-v, --version` | Show version |

## Shell Completion

```bash
# Bash
trokky completion bash > /etc/bash_completion.d/trokky

# Zsh
trokky completion zsh > "${fpath[1]}/_trokky"

# Fish
trokky completion fish > ~/.config/fish/completions/trokky.fish
```

## License

MIT
