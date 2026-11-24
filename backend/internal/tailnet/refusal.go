// Source: docklet-next/issues/599
// Revision: 22319d6f16ef401b1c247c7a4eae599676564095
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original bounded provider and tsnet refusal classification.

package tailnet

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	dto "github.com/florianfinn/docklet-next/types/tailnet"
	"tailscale.com/client/tailscale"
)

var refusedSentence = regexp.MustCompile(`requested tags \[([^\[\]\r\n]*)\] are invalid or not permitted`)
var safeTag = regexp.MustCompile(`^tag:[a-z0-9-]+$`)

// Only tag tokens leave the provider boundary; neither surrounding text nor a full error is returned.
func refusedTags(message string, exact bool) ([]string, bool) {
	if len(message) > maxProviderBytes {
		return nil, false
	}
	match := refusedSentence.FindStringSubmatch(message)
	if match == nil || exact && match[0] != message {
		return nil, false
	}
	var tags []string
	for _, token := range strings.Fields(match[1]) {
		if safeTag.MatchString(token) {
			tags = append(tags, token)
			if len(tags) == 8 {
				break
			}
		}
	}
	return tags, true
}

func classifyMint(status int, body []byte, readErr error, fallback *dto.Problem) *dto.Problem {
	if status != http.StatusBadRequest || readErr != nil {
		return fallback
	}
	var response struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &response) != nil {
		return fallback
	}
	if tags, ok := refusedTags(response.Message, true); ok {
		return &dto.Problem{Code: dto.CodeTagsNotPermitted, Status: fallback.Status, ProviderStatus: status, RefusedTags: tags}
	}
	return fallback
}

// tsnet wraps some control responses as text. Prefer the typed response when it survives wrapping.
func joinRefusal(err error) *dto.Problem {
	if err == nil {
		return nil
	}
	var response tailscale.ErrResponse
	var pointer *tailscale.ErrResponse
	typed := errors.As(err, &response)
	if !typed && errors.As(err, &pointer) && pointer != nil {
		response, typed = *pointer, true
	}
	if typed {
		if response.Status == http.StatusBadRequest {
			if tags, ok := refusedTags(response.Message, true); ok {
				return &dto.Problem{Code: dto.CodeTagsNotPermitted, Status: http.StatusConflict, ProviderStatus: response.Status, RefusedTags: tags}
			}
		}
		if response.Status == 400 || response.Status == 401 || response.Status == 403 {
			return rejected(response.Status)
		}
		return nil
	}
	text := err.Error()
	if tags, ok := refusedTags(text, false); ok {
		return &dto.Problem{Code: dto.CodeTagsNotPermitted, Status: http.StatusConflict, RefusedTags: tags}
	}
	if len(text) > maxProviderBytes {
		return nil
	}
	msg := strings.ToLower(text)
	if strings.Contains(msg, "backend:") {
		return rejected(0)
	}
	if strings.Contains(msg, "resolving auth key") {
		for _, marker := range []string{"401", "403", "400", "unauthorized", "invalid", "forbidden"} {
			if strings.Contains(msg, marker) {
				return rejected(0)
			}
		}
	}
	return nil
}
