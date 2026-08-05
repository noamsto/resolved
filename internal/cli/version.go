package cli

import (
	"fmt"

	"github.com/alecthomas/kong"
)

// version is overridden at build time via -ldflags "-X ...cli.version=...".
var version = "dev"

// VersionCmd prints the version.
type VersionCmd struct{}

func (VersionCmd) Run(kctx *kong.Context) error {
	fmt.Fprintln(kctx.Stdout, version)
	return nil
}
