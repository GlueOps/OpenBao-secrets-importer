# OpenBao Secrets Importer

A CLI tool to export secrets from various sources (AWS Secrets Manager, etc.) and import them into OpenBao (HashiCorp Vault fork) KV v2 secrets engine.

## Features

- **Export/Import Workflow**: Explicit two-step process with intermediate JSON file
- **Pluggable Sources**: Extensible architecture for adding new secret sources
- **Path Filtering**: Include/exclude patterns with glob syntax
- **Conflict Resolution**: Skip existing, overwrite all, or interactive per-secret prompts
- **Custom Headers**: Support for WAF/proxy authentication headers
- **Parallel Import**: Configurable worker pool for faster imports
- **Dry Run Mode**: Preview operations without making changes
asdasdasd
## Installation

### From Source

```bash
go install github.com/GlueOps/openbao-secrets-importer/cmd/openbao-secrets-importer@latest
```

### Build Locally

```bash
git clone https://github.com/GlueOps/openbao-secrets-importer.git
cd openbao-secrets-importer
go build -o openbao-secrets-importer ./cmd/openbao-secrets-importer
```

## Workflow

1. **List** secrets from source to preview what will be exported
2. **Export** secrets to a JSON file
3. **Validate** the export file (optional)
4. **Import** secrets from the file to OpenBao

## Usage

### List Secrets

Preview secrets from a source without fetching values:

```bash
# List all secrets from AWS Secrets Manager
openbao-secrets-importer list --source aws-secrets-manager

# List with filters
openbao-secrets-importer list --source aws-secrets-manager \
  --include "prod/**" \
  --exclude "**/temp/*"
```

### Export Secrets

Export secrets to a JSON file:

```bash
# Export all secrets
openbao-secrets-importer export \
  --source aws-secrets-manager \
  --output secrets.json

# Export with filters
openbao-secrets-importer export \
  --source aws-secrets-manager \
  --include "prod/**" \
  --exclude "**/temp/*" \
  --output secrets.json

# Dry run to preview
openbao-secrets-importer export \
  --source aws-secrets-manager \
  --output secrets.json \
  --dry-run
```

### Validate Export File

Check the export file before importing:

```bash
openbao-secrets-importer validate --input secrets.json
```

### Import Secrets

Import secrets from an export file to OpenBao:

```bash
# Basic import
openbao-secrets-importer import \
  --input secrets.json \
  --openbao-addr https://openbao.example.com:8200 \
  --openbao-token hvs.xxx \
  --mount secret

# Import with path prefix
openbao-secrets-importer import \
  --input secrets.json \
  --openbao-addr https://openbao.example.com:8200 \
  --openbao-token hvs.xxx \
  --mount secret \
  --path-prefix "aws-imported/"

# Import with custom headers (for WAF/proxy)
openbao-secrets-importer import \
  --input secrets.json \
  --openbao-addr https://openbao.example.com:8200 \
  --openbao-token hvs.xxx \
  --mount secret \
  --header "X-Custom-Auth: token" \
  --header "X-Forwarded-For: internal"

# Interactive mode
openbao-secrets-importer import \
  --input secrets.json \
  --openbao-addr https://openbao.example.com:8200 \
  --openbao-token hvs.xxx \
  --mount secret \
  --interactive

# Overwrite all existing secrets
openbao-secrets-importer import \
  --input secrets.json \
  --openbao-addr https://openbao.example.com:8200 \
  --openbao-token hvs.xxx \
  --mount secret \
  --overwrite-all
```

## AWS Configuration

The tool uses the standard AWS SDK credential chain:

- Environment variables: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`
- Shared credentials file (`~/.aws/credentials`)
- IAM role (if running on EC2/ECS/Lambda)

Set the region via:

- `--region` flag
- `AWS_REGION` environment variable

## Export File Format

The export file follows a versioned JSON schema:

```json
{
  "version": "1.0",
  "metadata": {
    "source": "aws-secrets-manager",
    "exported_at": "2025-12-04T10:30:00Z",
    "region": "us-east-1",
    "total_secrets": 3
  },
  "secrets": [
    {
      "path": "prod/myapp/database",
      "data": {
        "username": "admin",
        "password": "secret"
      },
      "metadata": {
        "source_id": "arn:aws:secretsmanager:...",
        "description": "Database credentials"
      }
    }
  ]
}
```

### Secret Value Handling

| AWS Secret Type | Handling |
|----------------|----------|
| JSON string | Parsed into individual key-value pairs |
| Plain text | Stored as `{<secret-name>: <value>}` |
| Binary | Base64 encoded, stored as `{<secret-name>: <base64-string>}` |

## Conflict Resolution

| Flag | Behavior |
|------|----------|
| `--skip-existing` | Skip secrets that already exist (default) |
| `--overwrite-all` | Overwrite all existing secrets without prompt |
| `--interactive` | Prompt per secret: Yes / No / Yes-to-all / No-to-all / Abort |

## Path Prefix

| Value | Result |
|-------|--------|
| *(omitted)* | Paths used as-is |
| `--path-prefix "aws/"` | Prepends `aws/` to all paths |
| `--path-prefix ""` or `--path-prefix "/"` | No prefix (root namespace) |

## Available Sources

- `aws-secrets-manager` - AWS Secrets Manager

## Adding New Sources

Implement the `Source` interface in `pkg/source/source.go`:

```go
type Source interface {
    Name() string
    Description() string
    Configure(ctx context.Context, opts map[string]interface{}) error
    List(ctx context.Context, patterns []string) ([]SecretInfo, error)
    Get(ctx context.Context, path string) (*Secret, error)
    Export(ctx context.Context, patterns []string) (<-chan *Secret, <-chan error)
}
```

Register the source in an `init()` function:

```go
func init() {
    source.Register("my-source", NewMySource)
}
```

## License

MIT
