package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"
)

// CLI is the full command grammar; kong parses os.Args into it. Field order is
// the order subcommands appear in --help.
type CLI struct {
	Scan    ScanCmd    `cmd:"" help:"Scan comments for stale GitHub references"`
	Check   CheckCmd   `cmd:"" help:"Print the status of a single GitHub reference"`
	Explore ExploreCmd `cmd:"" help:"Interactively browse stale GitHub references"`
	Version VersionCmd `cmd:"" help:"Print the version"`
}

// Execute runs the CLI. Errors exit with code 2 (tool error); subcommands set
// more specific exit codes via os.Exit themselves.
func Execute() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "resolved:", err)
		os.Exit(2)
	}
}

// run parses args and dispatches to the selected command. Split out of Execute
// so tests can drive the grammar without the process exit path.
func run(args []string, stdout, stderr io.Writer) error {
	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("resolved"),
		kong.Description("Find stale GitHub issue/PR references in code comments"),
		kong.Writers(stdout, stderr),
	)
	if err != nil {
		return err
	}
	kctx, err := parser.Parse(args)
	if err != nil {
		return err
	}
	return kctx.Run()
}
