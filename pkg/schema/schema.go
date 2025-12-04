// Package schema defines the export file contract/schema.
package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/GlueOps/openbao-secrets-importer/pkg/source"
)

// Version is the current schema version.
const Version = "1.0"

// ExportFile represents the structure of the export file.
type ExportFile struct {
	// Version is the schema version
	Version string `json:"version"`

	// Metadata contains information about the export
	Metadata ExportMetadata `json:"metadata"`

	// Secrets is the list of exported secrets
	Secrets []source.Secret `json:"secrets"`
}

// ExportMetadata contains metadata about the export operation.
type ExportMetadata struct {
	// Source is the source identifier (e.g., "aws-secrets-manager")
	Source string `json:"source"`

	// ExportedAt is when the export was performed
	ExportedAt time.Time `json:"exported_at"`

	// Region is the source region (optional, source-specific)
	Region string `json:"region,omitempty"`

	// IncludePatterns are the patterns used to include secrets
	IncludePatterns []string `json:"include_patterns,omitempty"`

	// ExcludePatterns are the patterns used to exclude secrets
	ExcludePatterns []string `json:"exclude_patterns,omitempty"`

	// TotalSecrets is the count of secrets in the export
	TotalSecrets int `json:"total_secrets"`
}

// NewExportFile creates a new ExportFile with the current version.
func NewExportFile(sourceName string) *ExportFile {
	return &ExportFile{
		Version: Version,
		Metadata: ExportMetadata{
			Source:     sourceName,
			ExportedAt: time.Now().UTC(),
		},
		Secrets: []source.Secret{},
	}
}

// AddSecret adds a secret to the export file.
func (e *ExportFile) AddSecret(secret *source.Secret) {
	e.Secrets = append(e.Secrets, *secret)
	e.Metadata.TotalSecrets = len(e.Secrets)
}

// Write writes the export file to the specified path.
func (e *ExportFile) Write(path string) error {
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal export file: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write export file: %w", err)
	}

	return nil
}

// ReadExportFile reads and parses an export file from the specified path.
func ReadExportFile(path string) (*ExportFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read export file: %w", err)
	}

	var export ExportFile
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, fmt.Errorf("failed to parse export file: %w", err)
	}

	return &export, nil
}

// Validate checks if the export file is valid according to the schema.
func (e *ExportFile) Validate() error {
	if e.Version == "" {
		return fmt.Errorf("missing required field: version")
	}

	if e.Version != Version {
		return fmt.Errorf("unsupported schema version: %s (expected %s)", e.Version, Version)
	}

	if e.Metadata.Source == "" {
		return fmt.Errorf("missing required field: metadata.source")
	}

	if e.Metadata.ExportedAt.IsZero() {
		return fmt.Errorf("missing required field: metadata.exported_at")
	}

	for i, secret := range e.Secrets {
		if secret.Path == "" {
			return fmt.Errorf("secret at index %d: missing required field: path", i)
		}
		if secret.Data == nil {
			return fmt.Errorf("secret at index %d (%s): missing required field: data", i, secret.Path)
		}
	}

	// Validate TotalSecrets matches actual count
	if e.Metadata.TotalSecrets != len(e.Secrets) {
		return fmt.Errorf("metadata.total_secrets (%d) does not match actual secret count (%d)",
			e.Metadata.TotalSecrets, len(e.Secrets))
	}

	return nil
}

// ValidateFile reads and validates an export file.
func ValidateFile(path string) (*ExportFile, error) {
	export, err := ReadExportFile(path)
	if err != nil {
		return nil, err
	}

	if err := export.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return export, nil
}
