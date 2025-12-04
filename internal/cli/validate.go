package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/GlueOps/openbao-secrets-importer/pkg/schema"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate an export file",
	Long: `Validate an export file against the schema before importing.

This checks:
  - Schema version compatibility
  - Required fields are present
  - Secret paths and data are valid
  - Metadata consistency

Examples:
  openbao-secrets-importer validate --input secrets.json`,
	RunE: runValidate,
}

var (
	validateInput string
)

func init() {
	validateCmd.Flags().StringVarP(&validateInput, "input", "f", "", "Input file path")

	validateCmd.MarkFlagRequired("input")

	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	export, err := schema.ValidateFile(validateInput)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	fmt.Println("✓ Export file is valid")
	fmt.Println()
	fmt.Printf("  File:           %s\n", validateInput)
	fmt.Printf("  Schema version: %s\n", export.Version)
	fmt.Printf("  Source:         %s\n", export.Metadata.Source)
	fmt.Printf("  Exported at:    %s\n", export.Metadata.ExportedAt.Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("  Total secrets:  %d\n", export.Metadata.TotalSecrets)

	if export.Metadata.Region != "" {
		fmt.Printf("  Region:         %s\n", export.Metadata.Region)
	}

	if len(export.Metadata.IncludePatterns) > 0 {
		fmt.Printf("  Include:        %v\n", export.Metadata.IncludePatterns)
	}

	if len(export.Metadata.ExcludePatterns) > 0 {
		fmt.Printf("  Exclude:        %v\n", export.Metadata.ExcludePatterns)
	}

	// Show first few secret paths as preview
	if len(export.Secrets) > 0 {
		fmt.Println()
		fmt.Println("  Secret paths (first 10):")
		limit := 10
		if len(export.Secrets) < limit {
			limit = len(export.Secrets)
		}
		for i := 0; i < limit; i++ {
			fmt.Printf("    - %s\n", export.Secrets[i].Path)
		}
		if len(export.Secrets) > 10 {
			fmt.Printf("    ... and %d more\n", len(export.Secrets)-10)
		}
	}

	return nil
}
