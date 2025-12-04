// Package openbao provides the OpenBao/Vault KV v2 target client.
package openbao

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/vault/api"
)

// Client wraps the Vault API client for OpenBao KV v2 operations.
type Client struct {
	client    *api.Client
	mount     string
	headers   map[string]string
	mu        sync.RWMutex
}

// Config holds the configuration for the OpenBao client.
type Config struct {
	// Address is the OpenBao server address (e.g., "https://openbao.example.com:8200")
	Address string

	// Token is the authentication token
	Token string

	// Mount is the KV v2 mount path (e.g., "secret")
	Mount string

	// Headers are custom HTTP headers to add to all requests
	Headers map[string]string

	// TLSSkipVerify skips TLS certificate verification
	TLSSkipVerify bool

	// Timeout is the HTTP client timeout
	Timeout time.Duration
}

// NewClient creates a new OpenBao client.
func NewClient(cfg Config) (*Client, error) {
	// Create Vault API config
	apiConfig := api.DefaultConfig()
	apiConfig.Address = cfg.Address

	if cfg.Timeout > 0 {
		apiConfig.Timeout = cfg.Timeout
	}

	// Configure TLS
	if cfg.TLSSkipVerify {
		apiConfig.HttpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	// Create the Vault client
	client, err := api.NewClient(apiConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	// Set token
	client.SetToken(cfg.Token)

	// Set custom headers
	if len(cfg.Headers) > 0 {
		for key, value := range cfg.Headers {
			client.AddHeader(key, value)
		}
	}

	return &Client{
		client:  client,
		mount:   cfg.Mount,
		headers: cfg.Headers,
	}, nil
}

// WriteSecret writes a secret to KV v2.
func (c *Client) WriteSecret(ctx context.Context, path string, data map[string]interface{}) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	kv := c.client.KVv2(c.mount)

	_, err := kv.Put(ctx, path, data)
	if err != nil {
		return fmt.Errorf("failed to write secret to %s: %w", path, err)
	}

	return nil
}

// WriteSecretCAS writes a secret using Check-And-Set (CAS).
// If cas is 0, the write will only succeed if the key doesn't exist.
func (c *Client) WriteSecretCAS(ctx context.Context, path string, data map[string]interface{}, cas int) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	kv := c.client.KVv2(c.mount)

	_, err := kv.Put(ctx, path, data, api.WithCheckAndSet(cas))
	if err != nil {
		return fmt.Errorf("failed to write secret to %s: %w", path, err)
	}

	return nil
}

// ReadSecret reads a secret from KV v2.
func (c *Client) ReadSecret(ctx context.Context, path string) (map[string]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	kv := c.client.KVv2(c.mount)

	secret, err := kv.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret from %s: %w", path, err)
	}

	if secret == nil || secret.Data == nil {
		return nil, nil
	}

	return secret.Data, nil
}

// SecretExists checks if a secret exists at the given path.
func (c *Client) SecretExists(ctx context.Context, path string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	kv := c.client.KVv2(c.mount)

	secret, err := kv.Get(ctx, path)
	if err != nil {
		// Check if it's a "secret not found" error
		if strings.Contains(err.Error(), "secret not found") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check secret at %s: %w", path, err)
	}

	return secret != nil, nil
}

// ListSecrets lists secrets at the given path using the logical client.
func (c *Client) ListSecrets(ctx context.Context, path string) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Use the logical client to list metadata
	listPath := fmt.Sprintf("%s/metadata/%s", c.mount, path)
	secret, err := c.client.Logical().ListWithContext(ctx, listPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets at %s: %w", path, err)
	}

	if secret == nil || secret.Data == nil {
		return []string{}, nil
	}

	keysRaw, ok := secret.Data["keys"]
	if !ok {
		return []string{}, nil
	}

	keysSlice, ok := keysRaw.([]interface{})
	if !ok {
		return []string{}, nil
	}

	result := make([]string, len(keysSlice))
	for i, k := range keysSlice {
		result[i] = fmt.Sprintf("%v", k)
	}

	return result, nil
}

// Health checks the OpenBao server health.
func (c *Client) Health(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	health, err := c.client.Sys().Health()
	if err != nil {
		return fmt.Errorf("failed to check OpenBao health: %w", err)
	}

	if health.Sealed {
		return fmt.Errorf("OpenBao is sealed")
	}

	return nil
}

// Address returns the configured OpenBao address.
func (c *Client) Address() string {
	return c.client.Address()
}

// Mount returns the configured KV mount path.
func (c *Client) Mount() string {
	return c.mount
}

// ParseHeaders parses header strings in "Key: Value" format.
func ParseHeaders(headerStrings []string) (map[string]string, error) {
	headers := make(map[string]string)

	for _, h := range headerStrings {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid header format: %s (expected 'Key: Value')", h)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("invalid header: empty key in %s", h)
		}
		headers[key] = value
	}

	return headers, nil
}
