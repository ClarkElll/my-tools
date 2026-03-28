#!/usr/bin/env bash

set -euo pipefail

tool_name="${1:-}"

if [[ -z "${tool_name}" ]]; then
  echo "usage: $0 <tool-name>" >&2
  exit 1
fi

if [[ ! "${tool_name}" =~ ^[a-z0-9][a-z0-9-]*$ ]]; then
  echo "tool name must match ^[a-z0-9][a-z0-9-]*$" >&2
  exit 1
fi

tool_dir="app/${tool_name}"

if [[ -e "${tool_dir}" ]]; then
  echo "${tool_dir} already exists" >&2
  exit 1
fi

mkdir -p "${tool_dir}"

cat > "${tool_dir}/main.go" <<EOF
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ClarkElll/my-tools/internal/cliutil"
)

func main() {
	fs := flag.NewFlagSet("${tool_name}", flag.ExitOnError)
	fs.Usage = cliutil.NewUsage(fs, cliutil.Tool{
		Name:        "${tool_name}",
		Description: "${tool_name} utility.",
		Invocation:  "go run ./app/${tool_name}",
	})

	fs.Parse(os.Args[1:])
	fmt.Println("${tool_name} is ready")
}
EOF

gofmt -w "${tool_dir}/main.go"
echo "created ${tool_dir}/main.go"
