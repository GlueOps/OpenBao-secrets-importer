package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/GlueOps/openbao-secrets-importer/pkg/filter"
	"github.com/GlueOps/openbao-secrets-importer/pkg/schema"
	"github.com/GlueOps/openbao-secrets-importer/pkg/source"
	"github.com/GlueOps/openbao-secrets-importer/pkg/source/aws"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export secrets from a source to a file",
	Long: `Export secrets from a source to a JSON file.
The export file follows a versioned schema and can be imported to OpenBao.

Examples:
  # Export all secrets from AWS Secrets Manager
  openbao-secrets-importer export --source aws-secrets-manager --output secrets.json

  # Export with filters
  openbao-secrets-importer export --source aws-secrets-manager \
    --include "prod/**" --exclude "**/temp/*" \
    --output secrets.json

  # Dry run to preview without writing
  openbao-secrets-importer export --source aws-secrets-manager --output secrets.json --dry-run`,
	RunE: runExport,
}

var (
	exportSource   string
	exportOutput   string
	exportIncludes []string
	exportExcludes []string
	exportRegion   string
	exportDryRun   bool
)

func init() {
	exportCmd.Flags().StringVarP(&exportSource, "source", "s", "", "Secret source (e.g., aws-secrets-manager)")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Output file path")
	exportCmd.Flags().StringArrayVarP(&exportIncludes, "include", "i", []string{}, "Include patterns (glob syntax, can be specified multiple times)")
	exportCmd.Flags().StringArrayVarP(&exportExcludes, "exclude", "e", []string{}, "Exclude patterns (glob syntax, can be specified multiple times)")
	exportCmd.Flags().StringVar(&exportRegion, "region", "", "AWS region (for aws-secrets-manager source)")
	exportCmd.Flags().BoolVar(&exportDryRun, "dry-run", false, "Preview export without writing to file")

	exportCmd.MarkFlagRequired("source")
	exportCmd.MarkFlagRequired("output")

	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get the source
	src, err := source.Get(exportSource)
	if err != nil {
		return fmt.Errorf("failed to get source: %w", err)
	}

	// Configure the source
	opts := make(map[string]interface{})
	if exportRegion != "" {
		opts["region"] = exportRegion
	}

	if err := src.Configure(ctx, opts); err != nil {
		return fmt.Errorf("failed to configure source: %w", err)
	}

	// Create path filter
	pathFilter, err := filter.NewPathFilter(exportIncludes, exportExcludes)
	if err != nil {
		return fmt.Errorf("invalid filter pattern: %w", err)
	}

	// Create export file
	exportFile := schema.NewExportFile(src.Name())
	exportFile.Metadata.IncludePatterns = exportIncludes
	exportFile.Metadata.ExcludePatterns = exportExcludes

	// Add region for AWS source
	if awsSrc, ok := src.(*aws.Source); ok {
		exportFile.Metadata.Region = awsSrc.Region()
	}

	// List secrets first
	fmt.Fprintf(os.Stderr, "Listing secrets from %s...\n", src.Name())

	patterns := exportIncludes
	if len(patterns) == 0 {
		patterns = []string{"**"}
	}

	infos, err := src.List(ctx, patterns)
	if err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	}

	// Filter with excludes
	var filteredPaths []string
	for _, info := range infos {
		if pathFilter.Matches(info.Path) {
			filteredPaths = append(filteredPaths, info.Path)
		}
	}

	if len(filteredPaths) == 0 {
		fmt.Println("No secrets found matching the specified patterns.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Found %d secrets to export.\n", len(filteredPaths))

	if exportDryRun {
		fmt.Println("\nDry run - secrets that would be exported:")
		for _, path := range filteredPaths {
			fmt.Printf("  %s\n", path)
		}
		return nil
	}

	// Export secrets
	fmt.Fprintf(os.Stderr, "Exporting secrets...\n")

	var errCount int
	for idx, path := range filteredPaths {
		fmt.Fprintf(os.Stderr, "\r  [%d/%d] Fetching %s...", idx+1, len(filteredPaths), path)

		secret, err := src.Get(ctx, path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n  Warning: failed to get secret %s: %v\n", path, err)
			errCount++
			continue
		}

		exportFile.AddSecret(secret)
	}
	fmt.Fprintf(os.Stderr, "\r  Exported %d secrets.                          \n", len(exportFile.Secrets))

	if errCount > 0 {
		fmt.Fprintf(os.Stderr, "  Warning: %d secrets failed to export.\n", errCount)
	}

	// Write export file
	if err := exportFile.Write(exportOutput); err != nil {
		return fmt.Errorf("failed to write export file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nExport complete: %s\n", exportOutput)
	fmt.Fprintf(os.Stderr, "  Total secrets: %d\n", exportFile.Metadata.TotalSecrets)
	fmt.Fprintf(os.Stderr, "  Schema version: %s\n", exportFile.Version)

	return nil
}
