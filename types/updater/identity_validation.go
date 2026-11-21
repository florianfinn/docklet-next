// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Validate identities at agent response boundaries. M35 #555: unavailable status may omit revisions.

package updater

import (
	"github.com/distribution/reference"
	"regexp"
)

var identityPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var imageIDPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func ValidIdentity(s string) bool { return identityPattern.MatchString(s) }

// Valid requires both revisions for an available status. An unavailable status
// with a known reason may omit them (agents before M35 answered external drift so);
// it is shown only and never authorises a mutation.
func (s BaselineStatus) Valid() bool {
	if !ValidIdentity(s.TargetIdentity) {
		return false
	}
	if s.Available || (s.Code != "update.config-drift" && s.Code != "update.baseline-unavailable") {
		return ValidIdentity(s.SavedRevision) && ValidIdentity(s.ObservedRevision)
	}
	return optionalIdentity(s.SavedRevision) && optionalIdentity(s.ObservedRevision)
}
func optionalIdentity(s string) bool { return s == "" || ValidIdentity(s) }
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
