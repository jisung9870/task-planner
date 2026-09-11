// Command tp is the task-planner entry point: a TUI when run bare, a CLI when
// given a subcommand.
package main

import (
	"os"

	"task-planner/internal/cli"
)

func main() { os.Exit(cli.Execute()) }
