// Source: getarcaneapp/arcane/cli/pkg/projects/cmd.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-20
// License: BSD-3-Clause
// Deviations: Extracted stream reader; require terminal completion and preserve uncoded diagnostics.

package projects

import (
	"bufio"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/florianfinn/docklet-next/cli/internal/output"
)

// printOperationStreamInternal consumes an NDJSON operation stream, printing
// raw docker CLI log lines to stdout. A terminal error frame is returned as an
// error. EOF before done is an interrupted operation; unknown frames (e.g. the activity envelope) are skipped.
func printOperationStreamInternal(body io.Reader) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var frame struct {
			Log   string `json:"log"`
			Error string `json:"error"`
			Done  bool   `json:"done"`
			Code  string `json:"code"`
		}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			continue
		}
		if frame.Error != "" {
			if frame.Code != "" || frame.Done {
				return errors.New(frame.Error)
			}
			_, _ = fmt.Fprintln(output.Stderr(), frame.Error)
		}
		if frame.Done {
			return nil
		}
		if frame.Log != "" {
			_, _ = fmt.Fprintln(output.Stdout(), frame.Log)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return errors.New("runtime operation stream interrupted before completion")
}
