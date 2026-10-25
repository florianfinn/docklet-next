// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original image and registry safety contract for issue #229.

package middleware

import (
	"bytes"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/labstack/echo/v5"
	"io"
	"net/http"
	"strings"
)

// checkImageUpgrade runs after user permission checks and before either remote
// transport. Replaying bounded JSON preserves the original authorized request.
func checkImageUpgrade(c *echo.Context) error {
	request := c.Request()
	path := request.URL.Path
	if request.Method != http.MethodPost || !strings.HasSuffix(path, "/system/upgrade") {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(request.Body, 64<<10))
	if err != nil || len(data) >= 64<<10 {
		return imageDTO.Failure("update.invalid-target", 400)
	}
	request.Body = io.NopCloser(bytes.NewReader(data))
	var input struct {
		TargetVersion string `json:"targetVersion"`
	}
	if json.Unmarshal(data, &input) != nil {
		return imageDTO.Failure("update.invalid-target", 400)
	}
	return imageDTO.ValidateAgentUpgradeTarget(input.TargetVersion, config.Version)
}
