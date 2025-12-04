package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"

	"github.com/GlueOps/openbao-secrets-importer/pkg/schema"
	"github.com/GlueOps/openbao-secrets-importer/pkg/source"
	"github.com/GlueOps/openbao-secrets-importer/pkg/target/openbao"
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import secrets from a file to OpenBao",
	Long: `Import secrets from an export file to OpenBao KV v2 secrets engine.

Conflict Resolution:
  --skip-existing   Skip secrets that already exist (default)
  --overwrite-all   Overwrite all existing secrets without prompting
  --interactive     Prompt for each secret (Yes/No/Yes-to-all/No-to-all/Abort)

Examples:
  # Basic import
  openbao-secrets-importer import \
    --input secrets.json \
    --openbao-addr https://openbao:8200 \
    --openbao-token hvs.xxx \
    --mount secret

  # Import with path prefix
  openbao-secrets-importer import \
    --input secrets.json \
    --openbao-addr https://openbao:8200 \
    --openbao-token hvs.xxx \
    --mount secret \
    --path-prefix "aws-imported/"

  # Import with custom headers for WAF/proxy
  openbao-secrets-importer import \
    --input secrets.json \
    --openbao-addr https://openbao:8200 \
    --openbao-token hvs.xxx \
    --mount secret \
    --header "X-Custom-Auth: token" \
    --header "X-Forwarded-For: internal"

  # Interactive mode with per-secret prompts
  openbao-secrets-importer import \
    --input secrets.json \
    --openbao-addr https://openbao:8200 \
    --openbao-token hvs.xxx \
    --mount secret \
    --interactive`,
	RunE: runImport,
}

var (
	importInput        string
	importOpenBaoAddr  string
	importOpenBaoToken string
	importMount        string
	importHeaders      []string
	importPathPrefix   string
	importSkipExisting bool
	importOverwriteAll bool
	importInteractive  bool
	importParallelism  int
	importDryRun       bool
	importTLSSkipVerify bool
)

func init() {
	importCmd.Flags().StringVarP(&importInput, "input", "f", "", "Input file path")
	importCmd.Flags().StringVar(&importOpenBaoAddr, "openbao-addr", "", "OpenBao server address (e.g., https://openbao:8200)")
	importCmd.Flags().StringVar(&importOpenBaoToken, "openbao-token", "", "OpenBao authentication token")
	importCmd.Flags().StringVar(&importMount, "mount", "secret", "KV v2 mount path")
	importCmd.Flags().StringArrayVar(&importHeaders, "header", []string{}, "Custom HTTP header (can be specified multiple times, format: 'Key: Value')")
	importCmd.Flags().StringVar(&importPathPrefix, "path-prefix", "", "Prefix to prepend to all secret paths")
	importCmd.Flags().BoolVar(&importSkipExisting, "skip-existing", true, "Skip secrets that already exist")
	importCmd.Flags().BoolVar(&importOverwriteAll, "overwrite-all", false, "Overwrite all existing secrets without prompting")
	importCmd.Flags().BoolVar(&importInteractive, "interactive", false, "Prompt for each secret")
	importCmd.Flags().IntVar(&importParallelism, "parallelism", 5, "Number of parallel import workers")
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Preview import without writing to OpenBao")
	importCmd.Flags().BoolVar(&importTLSSkipVerify, "tls-skip-verify", false, "Skip TLS certificate verification")

	importCmd.MarkFlagRequired("input")
	importCmd.MarkFlagRequired("openbao-addr")
	importCmd.MarkFlagRequired("openbao-token")

	rootCmd.AddCommand(importCmd)
}

// ImportConfirmation represents the user's choice for a secret.
type ImportConfirmation int

const (
	ConfirmYes ImportConfirmation = iota
	ConfirmNo
	ConfirmYesToAll
	ConfirmNoToAll
	ConfirmAbort
)

// ImportResult tracks the result of an import operation.
type ImportResult struct {
	Path    string
	Success bool
	Skipped bool
	Error   error
}

func runImport(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Validate flags
	if importOverwriteAll && importInteractive {
		return fmt.Errorf("--overwrite-all and --interactive cannot be used together")
	}

	if importOverwriteAll {
		importSkipExisting = false
	}

	// Read and validate export file
	fmt.Fprintf(os.Stderr, "Reading export file: %s\n", importInput)
	export, err := schema.ValidateFile(importInput)
	if err != nil {
		return fmt.Errorf("failed to read/validate export file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "  Found %d secrets to import\n", len(export.Secrets))

	// Parse custom headers
	headers, err := openbao.ParseHeaders(importHeaders)
	if err != nil {
		return fmt.Errorf("invalid header: %w", err)
	}

	// Normalize path prefix
	pathPrefix := normalizePathPrefix(importPathPrefix)

	if importDryRun {
		return runDryRun(export, pathPrefix)
	}

	// Create OpenBao client
	fmt.Fprintf(os.Stderr, "Connecting to OpenBao: %s\n", importOpenBaoAddr)
	client, err := openbao.NewClient(openbao.Config{
		Address:       importOpenBaoAddr,
		Token:         importOpenBaoToken,
		Mount:         importMount,
		Headers:       headers,
		TLSSkipVerify: importTLSSkipVerify,
		Timeout:       30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	// Check connection
	if err := client.Health(ctx); err != nil {
		return fmt.Errorf("failed to connect to OpenBao: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  Connected successfully\n")

	// Run import
	if importInteractive {
		return runInteractiveImport(ctx, client, export, pathPrefix)
	}

	return runParallelImport(ctx, client, export, pathPrefix)
}

func normalizePathPrefix(prefix string) string {
	if prefix == "" || prefix == "/" {
		return ""
	}
	// Ensure prefix ends with /
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	// Remove leading /
	prefix = strings.TrimPrefix(prefix, "/")
	return prefix
}

func runDryRun(export *schema.ExportFile, pathPrefix string) error {
	fmt.Println("\nDry run - secrets that would be imported:")
	fmt.Println()

	for _, secret := range export.Secrets {
		destPath := pathPrefix + secret.Path
		keys := getSecretKeys(secret.Data)
		fmt.Printf("  %s -> %s\n", secret.Path, destPath)
		fmt.Printf("    Keys: %s\n", strings.Join(keys, ", "))
	}

	fmt.Printf("\nTotal: %d secrets\n", len(export.Secrets))
	return nil
}

func runInteractiveImport(ctx context.Context, client *openbao.Client, export *schema.ExportFile, pathPrefix string) error {
	fmt.Println("\nStarting interactive import...")
	fmt.Println()

	var imported, skipped, failed int
	confirmAll := false
	skipAll := false

	for i, secret := range export.Secrets {
		destPath := pathPrefix + secret.Path

		// Check if already decided for all
		if skipAll {
			skipped++
			continue
		}

		if !confirmAll {
			// Check if exists
			exists, err := client.SecretExists(ctx, destPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to check if secret exists: %v\n", err)
			}

			// Prompt user
			confirmation, err := promptImport(i+1, len(export.Secrets), secret, destPath, exists)
			if err != nil {
				return fmt.Errorf("prompt failed: %w", err)
			}

			switch confirmation {
			case ConfirmNo:
				fmt.Printf("  Skipped\n")
				skipped++
				continue
			case ConfirmYesToAll:
				confirmAll = true
			case ConfirmNoToAll:
				skipAll = true
				skipped++
				continue
			case ConfirmAbort:
				fmt.Println("\nImport aborted by user.")
				fmt.Printf("  Imported: %d\n", imported)
				fmt.Printf("  Skipped:  %d\n", skipped)
				return nil
			}
		}

		// Import the secret
		if err := client.WriteSecret(ctx, destPath, secret.Data); err != nil {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
			failed++
			continue
		}

		fmt.Printf("  ✓ Imported\n")
		imported++
	}

	fmt.Println()
	fmt.Println("Import complete:")
	fmt.Printf("  Imported: %d\n", imported)
	fmt.Printf("  Skipped:  %d\n", skipped)
	fmt.Printf("  Failed:   %d\n", failed)

	return nil
}

func promptImport(current, total int, secret source.Secret, destPath string, exists bool) (ImportConfirmation, error) {
	// Display secret info
	fmt.Printf("[%d/%d] Secret: %s\n", current, total, secret.Path)
	fmt.Printf("  Destination: %s\n", destPath)
	fmt.Printf("  Keys: %s\n", strings.Join(getSecretKeys(secret.Data), ", "))
	if secret.Metadata.Description != "" {
		fmt.Printf("  Description: %s\n", secret.Metadata.Description)
	}
	if exists {
		fmt.Printf("  ⚠️  Secret already exists at destination\n")
	}

	options := []string{
		"Yes",
		"No",
		"Yes to all remaining",
		"No to all remaining",
		"Abort import",
	}

	var selection string
	prompt := &survey.Select{
		Message: "Import this secret?",
		Options: options,
	}

	if err := survey.AskOne(prompt, &selection); err != nil {
		return ConfirmAbort, err
	}

	switch selection {
	case "Yes":
		return ConfirmYes, nil
	case "No":
		return ConfirmNo, nil
	case "Yes to all remaining":
		return ConfirmYesToAll, nil
	case "No to all remaining":
		return ConfirmNoToAll, nil
	default:
		return ConfirmAbort, nil
	}
}

func runParallelImport(ctx context.Context, client *openbao.Client, export *schema.ExportFile, pathPrefix string) error {
	fmt.Fprintf(os.Stderr, "\nImporting secrets with %d workers...\n", importParallelism)

	var (
		imported int64
		skipped  int64
		failed   int64
		wg       sync.WaitGroup
	)

	// Create work channel
	work := make(chan source.Secret, len(export.Secrets))
	results := make(chan ImportResult, len(export.Secrets))

	// Start workers
	for i := 0; i < importParallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for secret := range work {
				result := importSecret(ctx, client, secret, pathPrefix)
				results <- result
			}
		}()
	}

	// Send work
	go func() {
		for _, secret := range export.Secrets {
			work <- secret
		}
		close(work)
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results
	for result := range results {
		if result.Skipped {
			atomic.AddInt64(&skipped, 1)
		} else if result.Success {
			atomic.AddInt64(&imported, 1)
		} else {
			atomic.AddInt64(&failed, 1)
			fmt.Fprintf(os.Stderr, "  Error importing %s: %v\n", result.Path, result.Error)
		}

		total := atomic.LoadInt64(&imported) + atomic.LoadInt64(&skipped) + atomic.LoadInt64(&failed)
		fmt.Fprintf(os.Stderr, "\r  Progress: %d/%d", total, len(export.Secrets))
	}

	fmt.Fprintf(os.Stderr, "\n\n")
	fmt.Println("Import complete:")
	fmt.Printf("  Imported: %d\n", imported)
	fmt.Printf("  Skipped:  %d\n", skipped)
	fmt.Printf("  Failed:   %d\n", failed)

	if failed > 0 {
		return fmt.Errorf("%d secrets failed to import", failed)
	}

	return nil
}

func importSecret(ctx context.Context, client *openbao.Client, secret source.Secret, pathPrefix string) ImportResult {
	destPath := pathPrefix + secret.Path

	result := ImportResult{
		Path: destPath,
	}

	// Check if exists when skip-existing is enabled
	if importSkipExisting && !importOverwriteAll {
		exists, err := client.SecretExists(ctx, destPath)
		if err != nil {
			result.Error = fmt.Errorf("failed to check existence: %w", err)
			return result
		}
		if exists {
			result.Skipped = true
			result.Success = true
			return result
		}
	}

	// Write the secret
	if err := client.WriteSecret(ctx, destPath, secret.Data); err != nil {
		result.Error = err
		return result
	}

	result.Success = true
	return result
}

func getSecretKeys(data map[string]interface{}) []string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	return keys
}

// filepath.Base is imported but we need to reference it
var _ = filepath.Base
