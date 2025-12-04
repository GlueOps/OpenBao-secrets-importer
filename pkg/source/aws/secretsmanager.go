// Package aws provides the AWS Secrets Manager source implementation.
package aws

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/GlueOps/openbao-secrets-importer/pkg/filter"
	"github.com/GlueOps/openbao-secrets-importer/pkg/source"
)

const (
	// SourceName is the identifier for this source.
	SourceName = "aws-secrets-manager"
)

func init() {
	// Register this source with the default registry
	source.Register(SourceName, NewSource)
}

// Source implements the source.Source interface for AWS Secrets Manager.
type Source struct {
	client *secretsmanager.Client
	region string
}

// NewSource creates a new AWS Secrets Manager source.
func NewSource() source.Source {
	return &Source{}
}

// Name returns the source identifier.
func (s *Source) Name() string {
	return SourceName
}

// Description returns a human-readable description.
func (s *Source) Description() string {
	return "AWS Secrets Manager"
}

// Configure initializes the source with AWS credentials and region.
// Options:
//   - region: AWS region (optional, falls back to AWS_REGION env var)
//
// AWS credentials are loaded from the default credential chain:
//   - Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN)
//   - Shared credentials file
//   - IAM role (if running on EC2/ECS/Lambda)
func (s *Source) Configure(ctx context.Context, opts map[string]interface{}) error {
	var cfgOpts []func(*config.LoadOptions) error

	// Get region from options or environment
	if region, ok := opts["region"].(string); ok && region != "" {
		s.region = region
		cfgOpts = append(cfgOpts, config.WithRegion(region))
	}

	// Load AWS configuration using default credential chain
	cfg, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Store the resolved region
	if s.region == "" {
		s.region = cfg.Region
	}

	s.client = secretsmanager.NewFromConfig(cfg)
	return nil
}

// List returns information about secrets matching the given patterns.
func (s *Source) List(ctx context.Context, patterns []string) ([]source.SecretInfo, error) {
	if s.client == nil {
		return nil, fmt.Errorf("source not configured")
	}

	// Create filter
	pathFilter, err := filter.NewPathFilter(patterns, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	var secrets []source.SecretInfo
	paginator := secretsmanager.NewListSecretsPaginator(s.client, &secretsmanager.ListSecretsInput{})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list secrets: %w", err)
		}

		for _, secret := range page.SecretList {
			name := aws.ToString(secret.Name)

			// Apply filter
			if !pathFilter.Matches(name) {
				continue
			}

			info := source.SecretInfo{
				Path:        name,
				Description: aws.ToString(secret.Description),
				Tags:        make(map[string]string),
			}

			for _, tag := range secret.Tags {
				info.Tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
			}

			secrets = append(secrets, info)
		}
	}

	return secrets, nil
}

// Get retrieves a single secret by path.
func (s *Source) Get(ctx context.Context, path string) (*source.Secret, error) {
	if s.client == nil {
		return nil, fmt.Errorf("source not configured")
	}

	result, err := s.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(path),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s: %w", path, err)
	}

	secret := &source.Secret{
		Path: path,
		Metadata: source.SecretMetadata{
			SourceID: aws.ToString(result.ARN),
		},
	}

	// Handle binary vs string secrets
	if result.SecretBinary != nil {
		// Binary secret: base64 encode and use secret name as key
		encoded := base64.StdEncoding.EncodeToString(result.SecretBinary)
		secretName := filepath.Base(path)
		secret.Data = map[string]interface{}{secretName: encoded}
	} else if result.SecretString != nil {
		secretString := aws.ToString(result.SecretString)

		// Try to parse as JSON
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(secretString), &data); err != nil {
			// Not JSON: use secret name as key, value as the string
			secretName := filepath.Base(path)
			secret.Data = map[string]interface{}{secretName: secretString}
		} else {
			// Valid JSON: use parsed key-value pairs
			secret.Data = data
		}
	}

	// Get additional metadata
	descResult, err := s.client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(path),
	})
	if err == nil {
		secret.Metadata.Description = aws.ToString(descResult.Description)
		secret.Metadata.Tags = make(map[string]string)
		for _, tag := range descResult.Tags {
			secret.Metadata.Tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
		}
		if descResult.CreatedDate != nil {
			secret.Metadata.CreatedAt = descResult.CreatedDate
		}
		if descResult.LastChangedDate != nil {
			secret.Metadata.UpdatedAt = descResult.LastChangedDate
		}
	}

	return secret, nil
}

// Export retrieves all secrets matching the given patterns.
func (s *Source) Export(ctx context.Context, patterns []string) (<-chan *source.Secret, <-chan error) {
	secretChan := make(chan *source.Secret)
	errChan := make(chan error, 1)

	go func() {
		defer close(secretChan)
		defer close(errChan)

		// List all secrets matching patterns
		infos, err := s.List(ctx, patterns)
		if err != nil {
			errChan <- err
			return
		}

		// Use a worker pool for fetching secrets
		const workers = 5
		var wg sync.WaitGroup
		pathChan := make(chan string)

		// Start workers
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for path := range pathChan {
					secret, err := s.Get(ctx, path)
					if err != nil {
						// Log error but continue
						select {
						case errChan <- err:
						default:
						}
						continue
					}
					select {
					case secretChan <- secret:
					case <-ctx.Done():
						return
					}
				}
			}()
		}

		// Send paths to workers
		for _, info := range infos {
			select {
			case pathChan <- info.Path:
			case <-ctx.Done():
				close(pathChan)
				return
			}
		}
		close(pathChan)

		// Wait for all workers to complete
		wg.Wait()
	}()

	return secretChan, errChan
}

// Region returns the configured AWS region.
func (s *Source) Region() string {
	return s.region
}
