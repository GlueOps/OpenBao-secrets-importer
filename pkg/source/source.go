// Package source defines the interface for secret sources.
package source

import (
	"context"
	"time"
)

// Secret represents a single secret from any source.
type Secret struct {
	// Path is the hierarchical path (e.g., "myapp/db/credentials")
	Path string `json:"path"`

	// Data contains the key-value pairs of the secret
	Data map[string]interface{} `json:"data"`

	// Metadata from the source system
	Metadata SecretMetadata `json:"metadata,omitempty"`
}

// SecretMetadata contains optional metadata about a secret from the source.
type SecretMetadata struct {
	// SourceID is the unique identifier in the source system (e.g., ARN for AWS)
	SourceID string `json:"source_id,omitempty"`

	// Description is an optional description of the secret
	Description string `json:"description,omitempty"`

	// Tags are key-value tags from the source
	Tags map[string]string `json:"tags,omitempty"`

	// CreatedAt is when the secret was created in the source
	CreatedAt *time.Time `json:"created_at,omitempty"`

	// UpdatedAt is when the secret was last updated in the source
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// SecretInfo contains basic information about a secret without its value.
// Used for listing secrets.
type SecretInfo struct {
	// Path is the hierarchical path of the secret
	Path string

	// Description is an optional description
	Description string

	// Tags are key-value tags from the source
	Tags map[string]string
}

// Source is the interface that all secret sources must implement.
type Source interface {
	// Name returns the source identifier (e.g., "aws-secrets-manager")
	Name() string

	// Description returns a human-readable description of the source
	Description() string

	// Configure initializes the source with provided options.
	// Options are source-specific (e.g., region for AWS).
	Configure(ctx context.Context, opts map[string]interface{}) error

	// List returns information about secrets matching the given patterns.
	// Patterns support glob syntax (e.g., "myapp/*", "**").
	// If patterns is empty, all secrets are returned.
	List(ctx context.Context, patterns []string) ([]SecretInfo, error)

	// Get retrieves a single secret by path.
	Get(ctx context.Context, path string) (*Secret, error)

	// Export retrieves all secrets matching the given patterns.
	// Returns a channel of secrets and a channel of errors.
	// The caller should consume both channels until they are closed.
	Export(ctx context.Context, patterns []string) (<-chan *Secret, <-chan error)
}

// SourceFactory creates new Source instances.
type SourceFactory func() Source
