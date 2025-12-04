# CLAUDE.md - AI Assistant Instructions

This document provides context and guidelines for AI assistants working on this codebase.

## Project Overview

**OpenBao Secrets Importer** is a CLI tool that exports secrets from various sources (AWS Secrets Manager, etc.) and imports them into OpenBao (a HashiCorp Vault fork) KV v2 secrets engine.

### Key Design Decisions

1. **Explicit Export/Import Workflow**: Secrets are first exported to a JSON file, then imported separately. No direct sync mode.

2. **Pluggable Source Architecture**: Sources implement the `Source` interface in `pkg/source/source.go` and register via `init()` functions.

3. **Secret Value Handling**:
   - JSON secrets → parsed into individual key-value pairs
   - Plain text secrets → stored as `{secretName: value}`
   - Binary secrets → base64 encoded (no prefix)

4. **AWS Credentials**: Uses standard AWS SDK credential chain (env vars, shared credentials, IAM roles). No custom credential handling.

## Project Structure

```
├── cmd/openbao-secrets-importer/main.go    # Entry point
├── internal/cli/                            # CLI commands (Cobra)
│   ├── root.go       # Root command, version info
│   ├── list.go       # List secrets from source
│   ├── export.go     # Export to JSON file
│   ├── validate.go   # Validate export file schema
│   └── import.go     # Import to OpenBao
├── pkg/
│   ├── source/                              # Secret sources
│   │   ├── source.go           # Source interface definition
│   │   ├── registry.go         # Hard-coded source registry
│   │   └── aws/secretsmanager.go  # AWS Secrets Manager impl
│   ├── target/openbao/client.go   # OpenBao KV v2 client
│   ├── filter/filter.go           # Glob-based path filtering
│   └── schema/schema.go           # Export file JSON schema (v1.0)
└── .github/workflows/release.yml  # GoReleaser CI/CD
```

## Common Tasks

### Building

```bash
go build -o openbao-secrets-importer ./cmd/openbao-secrets-importer
```

### Running Tests

```bash
go test ./...
```

### Adding a New Secret Source

1. Create a new package under `pkg/source/<source-name>/`
2. Implement the `Source` interface from `pkg/source/source.go`
3. Register in `init()`: `source.Register("source-name", NewSource)`
4. Import the package in `internal/cli/root.go` for side-effect registration

### Export File Schema

Version: `1.0`

```json
{
  "version": "1.0",
  "metadata": {
    "source": "aws-secrets-manager",
    "exported_at": "2025-12-04T10:30:00Z",
    "region": "us-east-1",
    "include_patterns": ["prod/**"],
    "exclude_patterns": ["**/temp/*"],
    "total_secrets": 42
  },
  "secrets": [
    {
      "path": "prod/myapp/db",
      "data": {"username": "admin", "password": "secret"},
      "metadata": {"source_id": "arn:aws:...", "description": "..."}
    }
  ]
}
```

## CLI Flags Reference

### Import Command Key Flags

| Flag | Description |
|------|-------------|
| `--skip-existing` | Skip secrets that already exist (default: true) |
| `--overwrite-all` | Overwrite all existing secrets |
| `--interactive` | Prompt per secret (Yes/No/Yes-to-all/No-to-all/Abort) |
| `--header` | Repeatable custom HTTP header for WAF/proxy |
| `--path-prefix` | Prefix for all paths (empty or "/" = root) |
| `--parallelism` | Number of parallel workers (default: 5) |

### Export Command Key Flags

| Flag | Description |
|------|-------------|
| `--include` | Glob patterns to include (repeatable) |
| `--exclude` | Glob patterns to exclude (repeatable) |
| `--dry-run` | Preview without writing file |

## Dependencies

- `github.com/spf13/cobra` - CLI framework
- `github.com/aws/aws-sdk-go-v2` - AWS SDK
- `github.com/hashicorp/vault/api` - Vault/OpenBao client
- `github.com/AlecAivazis/survey/v2` - Interactive prompts
- `github.com/gobwas/glob` - Glob pattern matching

## Release Process

Releases are automated via GitHub Actions using GoReleaser:

1. Push a tag: `git tag v1.0.0 && git push origin v1.0.0`
2. GitHub Actions builds multi-arch binaries (Linux, macOS, Windows; amd64, arm64)
3. Creates GitHub Release with binaries and checksums
4. Release notes are auto-generated from commits

## Code Style

- Standard Go formatting (`gofmt`)
- Error wrapping with `fmt.Errorf("context: %w", err)`
- Context propagation for cancellation
- Structured logging to stderr, output to stdout

## Testing Considerations

- AWS source requires real credentials or mocks
- OpenBao target can be tested with a local dev server: `openbao server -dev`
- Export file validation can be unit tested with fixtures
