// Source: getarcaneapp/kit/acfs/cmd/main.go
// Revision: 3da7cfd43c4da62bc06fd384b7da5f109ce010fb
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package main

import (
	"context"
	"os"

	"go.getarcane.app/acfs/internal/cli"
)

func main() {
	os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
