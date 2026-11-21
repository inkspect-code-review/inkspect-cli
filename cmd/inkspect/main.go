package main

import (
	"fmt"
	"github.com/inkspect-code-review/inkspect-cli/internal/cli"
	"os"
)

func main() {
	if err := cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "inkspect:", err)
		os.Exit(1)
	}
}
