// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package middleware

import (
	"bytes"
	"encoding/json"
	"github.com/florianfinn/docklet-next/types/volume"
	"io"
	"net/http"
	"strings"
)

func validateProxiedVolumeEntry(request *http.Request) error {
	if !strings.Contains(request.URL.Path, "/volumes") {
		return nil
	}
	parts := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
	for i, p := range parts {
		if p == "volumes" && i+1 < len(parts) {
			if err := volume.ValidateName(parts[i+1]); err != nil {
				return err
			}
			break
		}
	}
	if request.Method != "POST" && request.Method != "PUT" {
		return nil
	}
	relevant := strings.HasSuffix(request.URL.Path, "/volumes") || strings.HasSuffix(request.URL.Path, "/volumes/prune") || strings.HasSuffix(request.URL.Path, "/rename") || strings.HasSuffix(request.URL.Path, "/restore") || strings.HasSuffix(request.URL.Path, "/binds/size")
	if !relevant || request.Body == nil {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(request.Body, 1024*1024+1))
	if err != nil {
		return err
	}
	request.Body = io.NopCloser(io.MultiReader(bytes.NewReader(data), request.Body))
	if len(data) > 1024*1024 {
		return nil
	}
	var body struct {
		Name          string
		NewVolumeName string
		Key           string
		Candidates    []struct{ Name string }
	}
	if json.Unmarshal(data, &body) != nil {
		return nil
	}
	for _, name := range []string{body.Name, body.NewVolumeName, body.Key} {
		if err := volume.ValidateName(name); err != nil {
			return err
		}
	}
	for _, c := range body.Candidates {
		if err := volume.ValidateName(c.Name); err != nil {
			return err
		}
	}
	return nil
}
