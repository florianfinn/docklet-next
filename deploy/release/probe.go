// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original offline release packaging and verification.

//go:build ignore

package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	if err := check(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func check() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("expected probe mode")
	}
	switch os.Args[1] {
	case "runtime":
		paths := map[string]os.FileMode{"/tmp": os.ModeSticky | 0777, "/app/data": 0700}
		if os.Args[2] == "hub" {
			paths["/var/lib/docklet/backups"] = 0700
		}
		for path, want := range paths {
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if info.Mode()&(os.ModePerm|os.ModeSticky) != want {
				return fmt.Errorf("runtime directory mode mismatch: %s", path)
			}
			if path != "/tmp" && info.Sys().(*syscall.Stat_t).Uid != 65532 {
				return fmt.Errorf("runtime directory owner mismatch: %s", path)
			}
		}
		processes, err := os.ReadDir("/proc")
		if err != nil {
			return err
		}
		found := false
		for _, process := range processes {
			if _, err := strconv.Atoi(process.Name()); err != nil {
				continue
			}
			status, err := os.ReadFile("/proc/" + process.Name() + "/status")
			if err != nil {
				continue
			}
			if !strings.Contains(string(status), "Name:\tdocklet-"+os.Args[2]+"\n") {
				continue
			}
			for _, line := range strings.Split(string(status), "\n") {
				f := strings.Fields(line)
				if len(f) == 5 && f[0] == "Uid:" && f[1] == "65532" && f[2] == "65532" && f[3] == "65532" && f[4] == "65532" {
					found = true
				}
			}
		}
		if !found {
			return fmt.Errorf("non-root re-exec child missing")
		}
		fmt.Println("private runtime directories, sticky tmp and numeric non-root re-exec passed")
	case "tls":
		cert, err := tls.LoadX509KeyPair("/tmp/server.crt", "/tmp/server.key")
		if err != nil {
			return err
		}
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "trusted") }))
		server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
		server.StartTLS()
		defer server.Close()
		client := &http.Client{Timeout: 5 * time.Second}
		response, err := client.Get(server.URL)
		if err != nil {
			return fmt.Errorf("default-root TLS: %w", err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		if string(body) != "trusted" {
			return fmt.Errorf("TLS response mismatch")
		}
		fmt.Println("default-root TLS lookup passed")
	default:
		return fmt.Errorf("unknown probe mode")
	}
	return nil
}
