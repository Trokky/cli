# Trokky CLI

Command-line tool for managing [Trokky CMS](https://github.com/Trokky/trokky) instances.

## Install

**macOS (Homebrew):**

```bash
brew install trokky/tap/trokky
```

**From GitHub Releases:**

Download the latest binary from [Releases](https://github.com/Trokky/cli/releases) and add it to your PATH.

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
| `generate-types` | Generate TypeScript types from schemas |

## Authentication

Trokky CLI supports three ways to provide credentials (in priority order):

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
trokky create my-site --template minimal --data postgres --media s3 --mail resend
```

Templates: `minimal`, `full`, `api-only`

## Global Flags

| Flag | Description |
|------|-------------|
| `--url <url>` | Trokky instance URL |
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
