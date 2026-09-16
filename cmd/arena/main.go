// Command arena is the single binary of Goyim Arena. Per the master plan,
// future subcommands include server, worker, migrate and explicitly approved
// operations; for now only version and help exist.
package main

import (
	"fmt"
	"os"
)

// version is the development version reported by `arena version` until the
// reproducible build metadata task (P01-T05) injects real values via -ldflags.
const version = "dev"

const usage = `arena is the command-line entrypoint of Goyim Arena.

Usage:

  arena <command> [arguments]

The commands are:

  version    show the arena version
  help       show this help

Run "arena <command> -h" for details about a command.`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "arena:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout *os.File) error {
	if len(args) == 0 {
		fmt.Fprintln(stdout, usage)
		return nil
	}

	switch args[0] {
	case "version":
		if len(args) > 1 {
			return fmt.Errorf("version takes no arguments (got %q)", args[1])
		}
		fmt.Fprintf(stdout, "arena version %s\n", version)
	case "help", "-h", "-help", "--help":
		if len(args) > 1 {
			return fmt.Errorf("help takes no arguments (got %q)", args[1])
		}
		fmt.Fprintln(stdout, usage)
	default:
		return fmt.Errorf("unknown command %q\n\n%s\n\nRun \"arena help\" for usage.", args[0], usage)
	}
	return nil
}
