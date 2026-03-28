package cliutil

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

func TestNewUsageIncludesDescriptionInvocationAndFlags(t *testing.T) {
	fs := flag.NewFlagSet("example", flag.ContinueOnError)
	var output bytes.Buffer
	fs.SetOutput(&output)
	fs.String("name", "world", "name to greet")

	NewUsage(fs, Tool{
		Name:        "example",
		Description: "Example utility.",
		Invocation:  "go run ./app/example",
	})()

	usage := output.String()
	for _, want := range []string{
		"Example utility.",
		"Usage:\n  go run ./app/example [flags]",
		"-name string",
	} {
		if !strings.Contains(usage, want) {
			t.Fatalf("usage output missing %q:\n%s", want, usage)
		}
	}
}

func TestNewUsageFallsBackToToolName(t *testing.T) {
	fs := flag.NewFlagSet("example", flag.ContinueOnError)
	var output bytes.Buffer
	fs.SetOutput(&output)

	NewUsage(fs, Tool{Name: "example"})()

	if !strings.Contains(output.String(), "Usage:\n  example [flags]") {
		t.Fatalf("usage output did not fall back to the tool name:\n%s", output.String())
	}
}
