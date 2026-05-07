# mymind CLI

> Go CLI client for the MyMind API — zero runtime dependencies, works with any agent.

## Install

```bash
git clone https://github.com/EyeSeeThru/mymind-cli.git
cd mymind-cli
go build -o ~/bin/mymind .
```

Or grab a release binary when available.

## Quick Start

```bash
# Authenticate
mymind auth login --kid <your-key-id> --secret <your-secret>

# Save a URL
mymind objects create --url https://example.com --tag reading

# Search
mymind search "design systems" --semantic --json

# List tags
mymind tags list --json | jq '.[].name'
```

## Auth

Credentials stored at `~/.config/mymind/config.json` (mode 0600), compatible with the [mm CLI](https://github.com/martinbavio/mm).

Or use env vars: `MYMIND_KID` and `MYMIND_SECRET`.

JWT: fresh HS256 per request, `exp = iat + 300`, bound to `method + path`.

## Commands

| Command | Description |
|---------|-------------|
| `mymind auth` | Login, logout, status, whoami |
| `mymind objects` | List, get, create, update, delete, restore, pin/unpin |
| `mymind objects blob\|screenshot\|thumbnail` | Binary streams (use `--output`) |
| `mymind objects tag add/remove` | Manage tags on objects |
| `mymind objects notes add/delete` | Manage notes |
| `mymind search` | Keyword and semantic search |
| `mymind tags` | List, get, create, delete tags |
| `mymind spaces` | List, get, create, delete spaces |
| `mymind convert` | Convert between text/markdown/prose |

## Flags

| Flag | Description |
|------|-------------|
| `--dry-run` | Preview request without sending (no credentials needed) |
| `--json` | Output raw JSON |
| `--jsonl` | Output newline-delimited JSON |
| `-v` | Verbose HTTP headers |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | User error |
| 2 | Auth error |
| 3 | Server error |
| 4 | Network error |
| 5 | Rate-limited (429, auto-retry) |

## Agent-Native Design

This CLI is designed for any AI agent that can run subprocess commands — Claude Code, OpenAI agents, Hermes Agent, OpenClaw, shell scripts, etc.

For Claude Code and Hermes Agent, a skill is included at `~/.hermes/skills/mymind/SKILL.md` that provides the full command reference.

Example agent usage:
```bash
# Save a link
mymind objects create --url "$URL" --tag "$TAG"

# Query with semantic search
mymind search "p5.js creative coding" --semantic --json

# Find similar objects
mymind search --similar-to "$OBJECT_ID"
```

## Credits

Inspired by [mm](https://github.com/martinbavio/mm) by Martin Bavio.
