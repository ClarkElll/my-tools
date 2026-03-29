package cliutil

import (
	"flag"
	"fmt"
)

// Tool describes the metadata used to render a command's help output.
type Tool struct {
	Name        string
	Description string
	Invocation  string
}

// NewUsage returns a flagset usage function with repository-wide formatting.
func NewUsage(fs *flag.FlagSet, tool Tool) func() {
	return func() {
		if tool.Description != "" {
			fmt.Fprintln(fs.Output(), tool.Description)
			fmt.Fprintln(fs.Output())
		}

		invocation := tool.Invocation
		if invocation == "" {
			invocation = tool.Name
		}
		if invocation == "" {
			invocation = fs.Name()
		}

		fmt.Fprintf(fs.Output(), "Usage:\n  %s [flags]\n\n", invocation)
		fmt.Fprintln(fs.Output(), "Flags:")
		fs.PrintDefaults()
	}
}
