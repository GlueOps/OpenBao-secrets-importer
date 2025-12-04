package main

import (
	"github.com/GlueOps/openbao-secrets-importer/internal/cli"
)

// Version information set by goreleaser or build flags
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cli.SetVersion(version, commit, date)
	cli.Execute()
}
