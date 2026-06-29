package main

import (
	"os"

	"github.com/adrianmross/oci-idm/internal/cli"
)

func main() {
	os.Exit(cli.RunWithName("oci-identity-apps", os.Args[1:], os.Stdout, os.Stderr))
}
