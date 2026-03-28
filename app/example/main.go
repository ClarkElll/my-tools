package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ClarkElll/my-tools/internal/cliutil"
)

func main() {
	fs := flag.NewFlagSet("example", flag.ExitOnError)
	name := fs.String("name", "world", "name to greet")
	upper := fs.Bool("upper", false, "render the message in upper case")
	fs.Usage = cliutil.NewUsage(fs, cliutil.Tool{
		Name:        "example",
		Description: "Example utility used as the starting point for new tools in this repository.",
		Invocation:  "go run ./app/example",
	})

	fs.Parse(os.Args[1:])

	target := strings.TrimSpace(*name)
	if target == "" {
		target = "world"
	}

	message := fmt.Sprintf("hello, %s", target)
	if *upper {
		message = strings.ToUpper(message)
	}

	fmt.Println(message)
}
