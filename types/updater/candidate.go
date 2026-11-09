// Source: docklet-next/issues/438
// Revision: 57e835f7cce5778fd3d164298e5b488e11abb71d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original optional manifest validation and trusted executor handoff.

package updater

import (
	"context"
	"regexp"
)

var manifestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func ValidManifest(digest string) bool { return manifestPattern.MatchString(digest) }

// VerifiedService admits only the scalar, one-service verified update shape.
func (r LifecycleRequest) VerifiedService() bool {
	if r.ExpectedManifest == "" {
		return r.Service == ""
	}
	return ValidManifest(r.ExpectedManifest) && r.Service != "" && len(r.Service) <= 256 && len(r.Services) == 0 && r.PointID == "" && !r.ReplaceConfiguration
}

func (r LifecycleRequest) SelectedServices() []string {
	if r.ExpectedManifest != "" {
		return []string{r.Service}
	}
	return r.Services
}

type expectedManifestKey struct{}

func WithExpectedManifest(ctx context.Context, digest string) context.Context {
	return context.WithValue(ctx, expectedManifestKey{}, digest)
}
func ExpectedManifestFromContext(ctx context.Context) string {
	value, _ := ctx.Value(expectedManifestKey{}).(string)
	return value
}
