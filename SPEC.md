# mymind CLI — Specification

## Overview

A Go CLI client for the MyMind API. Compiles to a single static binary with zero runtime dependencies.

## Building

```bash
cd ~/mymind-cli
go build -o ~/bin/mymind .
```

Dependencies: `github.com/spf13/cobra`, `github.com/golang-jwt/jwt/v5`, `github.com/rodaine/table`.

## Auth

**Credentials file:** `~/.config/mymind/config.json` (mode 0600)
**Format:**
```json
{
  "kid": "<key ID>",
  "secret": "<shared secret>"
}
```
**Env vars:** `MYMIND_KID` and `MYMIND_SECRET` override the config file.

JWT: fresh HS256 per request, `exp = iat + 300`. Claims: `iss` ("mymind"), `sub` (kid), `iat`, `exp`, `method`, `path` (no leading slash). Signing: HMAC-SHA256 of canonical signing string `"mymind.<method>.<path>.<iat>.<exp>"` with the base64-decoded secret.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | User error (bad args, not found, etc.) |
| 2 | Auth error (bad credentials, expired token) |
| 3 | Server error (5xx from API) |
| 4 | Network error (connection refused, DNS failure) |
| 5 | Rate-limited (429 response, retry-after respected) |

## Output Modes

- Default: pretty-printed tables or human-readable output
- `--json`: raw JSON
- `--jsonl`: newline-delimited JSON (one object per line)
- `--no-color`: disable ANSI color codes

## Global Flags

- `--dry-run`: print the HTTP request that *would* be made, without sending it
- `-v, --verbose`: print HTTP request/response headers and status codes

## Commands

### `mymind auth`

```
auth status      # check if credentials are stored
auth whoami      # verify credentials with a cheap API call
auth logout      # delete ~/.config/mymind/config.json
auth login --kid <key> --secret <secret>
```

### `mymind objects`

```
objects list [--query <text>] [--space <space-id>] [--ids <id,id>] [--limit 20] [--content-as markdown|html|prose]
objects get <id>
objects create (--url <url> | --content <text> | --file <path> | --stdin) [--title <title>] [--tag <name>]...
objects update <id> [--title <title>] [--summary <text>]
objects delete <id>
objects restore <id>
objects pin <id> [--position 0]
objects unpin <id>
objects get-content <id> [--format markdown|html|prose]
objects set-content <id> (--content <text> | --file <path> | --stdin) [--format markdown]
objects blob <id> [--output <path>]         # binary — always use --output or pipe
objects screenshot <id> [--output <path>]   # binary — always use --output or pipe
objects thumbnail <id> [--size 256x256] [--output <path>]
objects tag add <id> <tag1> [tag2] ...
objects tag remove <id> <tag1> [tag2] ...
objects notes add <id> --content <text>
objects notes delete <id> <note-id>
```

### `mymind search`

```
search <words...> [--semantic] [--rerank] [--semantic-boost 1-10] [--similar-to <id>] [--limit 20]
```

### `mymind tags`

```
tags list [--limit 20]
tags get <name>
tags create <name>
tags delete <name>
```

### `mymind spaces`

```
spaces list
spaces get <id>
spaces create <name>
spaces delete <id>
```

### `mymind convert`

```
convert --from text|markdown|prose --to text|markdown|prose [--content <text>] [--file <path>] [--stdin]
```

## HTTP

- Base URL: `https://api.mymind.com`
- User-Agent: `mymind-cli/<version>`
- Accept: `application/json` (overridden per endpoint)
- Automatic retry on 429 with `Retry-After` header; max 3 retries
- Timeout: 30 seconds per request
- No response caching

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | /objects | List/search objects |
| POST | /objects | Create object |
| GET | /objects/:id | Get object |
| PATCH | /objects/:id | Update title/summary |
| DELETE | /objects/:id | Soft-delete |
| POST | /objects/:id/restore | Restore |
| PUT | /objects/:id/pin | Pin |
| DELETE | /objects/:id/pin | Unpin |
| GET | /objects/:id/content | Content body |
| POST | /objects/:id/content | Set content |
| GET | /objects/:id/blob | Binary blob |
| GET | /objects/:id/screenshot | Screenshot |
| GET | /objects/:id/thumbnail | Thumbnail |
| POST | /objects/:id/tags | Add tags |
| DELETE | /objects/:id/tags | Remove tags |
| GET/POST/DELETE | /objects/:id/notes | Notes |
| GET | /search | Search |
| GET | /tags | List tags |
| GET/POST/DELETE | /tags/:name | Tag CRUD |
| GET | /spaces | List spaces |
| GET/POST/PATCH/DELETE | /spaces/:id | Space CRUD |
| POST/DELETE | /spaces/:id/members | Space members |
| POST | /convert | Format conversion |

## File Structure

```
~/mymind-cli/
├── SPEC.md
├── go.mod
├── main.go              # root command, flag parsing, error mapping
└── pkg/
    ├── auth/jwt.go      # credential loading, JWT signing
    ├── client/mymind.go # typed API methods
    ├── http/client.go   # HTTP transport, retry, dry-run
    ├── output/output.go # JSON/JSONL/pretty printing
    └── errors/errors.go # error types → exit codes
```
