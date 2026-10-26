// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Validate identities at agent response boundaries.

package updater

import (
	"github.com/distribution/reference"
	"regexp"
)

var identityPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var imageIDPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func ValidIdentity(s string) bool { return identityPattern.MatchString(s) }
func (s BaselineStatus) Valid() bool {
	return ValidIdentity(s.TargetIdentity) && ValidIdentity(s.SavedRevision) && ValidIdentity(s.ObservedRevision)
}
func validPointImage(s PointImage) bool {
	if s.Service == "" || s.Running && !s.Present || s.Digest != "" && !imageIDPattern.MatchString(s.Digest) {
		return false
	}
	if !s.Present {
		return s.ImageID == "" || imageIDPattern.MatchString(s.ImageID)
	}
	if !imageIDPattern.MatchString(s.ImageID) {
		return false
	}
	_, err := reference.ParseNormalizedNamed(s.Reference)
	return err == nil && s.Reference != ""
}
