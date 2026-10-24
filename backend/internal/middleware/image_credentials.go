// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original inline credential refusal for issue #229.

package middleware

import (
	"github.com/danielgtaylor/huma/v2"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"net/http"
	"net/url"
	"strings"
)

func imageCredentialSurface(path string) bool {
	return strings.HasSuffix(path, "/images/pull") || strings.Contains(path, "/image-updates/")
}

// InlineImageCredentials rejects presence, including empty values, without
// touching ordinary hub or manager Authorization authentication.
func InlineImageCredentials(path string, headers http.Header, query url.Values) error {
	if !imageCredentialSurface(path) {
		return nil
	}
	for key := range query {
		if imageDTO.InlineCredentialKey(key) {
			return imageDTO.Failure("registry.inline-credentials-not-allowed", 400)
		}
	}
	for key := range headers {
		if imageDTO.InlineCredentialKey(key) {
			return imageDTO.Failure("registry.inline-credentials-not-allowed", 400)
		}
	}
	return nil
}

func RejectInlineImageCredentials(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		headers := http.Header{}
		ctx.EachHeader(func(name, value string) { headers.Add(name, value) })
		requestURL := ctx.URL()
		if err := InlineImageCredentials(requestURL.Path, headers, requestURL.Query()); err != nil {
			problem := imageDTO.Neutral(err, "registry.inline-credentials-not-allowed", 400)
			ctx.SetHeader("Content-Type", "application/json")
			ctx.SetStatus(400)
			_ = api.Marshal(ctx.BodyWriter(), "application/json", problem)
			return
		}
		next(ctx)
	}
}
