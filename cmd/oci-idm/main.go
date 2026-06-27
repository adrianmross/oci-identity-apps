package main

import (
	"os"

	"github.com/adrianmross/oci-identity-apps/internal/cli"
)

func main() {
	os.Exit(cli.RunWithName("oci-idm", os.Args[1:], os.Stdout, os.Stderr))
}
