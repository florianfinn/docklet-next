//go:build ignore

// Verify formatting through the active Go toolchain, including container wrappers.
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
)

func main() {
	failed := false
	for _, path := range os.Args[1:] {
		if path == "--" {
			continue
		}
		data, err := os.ReadFile(path)
		if err == nil {
			var formatted []byte
			formatted, err = format.Source(data)
			if err == nil && !bytes.Equal(data, formatted) {
				fmt.Println(path + " [gofmt]")
				failed = true
			}
		}
		if err != nil {
			fmt.Println(path + " [gofmt-read-or-parse]")
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}
