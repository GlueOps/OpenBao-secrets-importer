// Package cli implements the command-line interface.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	// Register sources
	_ "github.com/GlueOps/openbao-secrets-importer/pkg/source/aws"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// rootCmd represents the base command.
var rootCmd = &cobra.Command{
	Use:   "openbao-secrets-importer",
	Short: "Import secrets from various sources into OpenBao",
	Long: `A CLI tool to export secrets from various sources (AWS Secrets Manager, etc.)
and import them into OpenBao (HashiCorp Vault fork) KV v2 secrets engine.

Workflow:
  1. List secrets from source to preview what will be exported
  2. Export secrets to a JSON file
  3. Validate the export file (optional)
  4. Import secrets from the file to OpenBao

Examples:
  # List secrets from AWS Secrets Manager
  openbao-secrets-importer list --source aws-secrets-manager --include "prod/**"

  # Export secrets to a file
  openbao-secrets-importer export --source aws-secrets-manager --output secrets.json

  # Validate the export file
  openbao-secrets-importer validate --input secrets.json

  # Import secrets to OpenBao
  openbao-secrets-importer import --input secrets.json --openbao-addr https://openbao:8200 --openbao-token hvs.xxx`,
}

// versionCmd shows version information.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("openbao-secrets-importer %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built:  %s\n", date)
	},
}

// sourcesCmd lists available sources.
var sourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "List available secret sources",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Available sources:")
		fmt.Println("  aws-secrets-manager  - AWS Secrets Manager")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(sourcesCmd)
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// SetVersion sets the version information for the CLI.
func SetVersion(v, c, d string) {
	version = v
	commit = c
	date = d
}
