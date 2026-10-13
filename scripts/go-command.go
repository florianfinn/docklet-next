//go:build ignore

// Unset service endpoints inside the toolchain environment, after a local
// container wrapper has used its own Docker transport to start Go.
package main

import (
	"os"
	"os/exec"
)

func main() {
	for _, name := range []string{"DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH"} {
		_ = os.Unsetenv(name)
	}
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "--work-off" {
		_ = os.Setenv("GOWORK", "off")
		args = args[1:]
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	cmd := exec.Command("go", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}
