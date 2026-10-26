// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original store-native image identity boundary.

package identity

import (
	"regexp"
	"strings"

	imageDTO "github.com/florianfinn/docklet-next/types/image"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Every role has a distinct type. Only constructors admit external values.
type LocalID struct{ value string }
type NativeKey struct{ value string }
type RegistryManifest struct{ value string }
type RegistryConfig struct{ value string }
type ExpectedLocalID struct{ value string }
type TopLevelDigest struct{ value string }
type Platform struct{ value ocispec.Platform }
type RegistryCandidate struct {
	manifest RegistryManifest
	config   RegistryConfig
	platform Platform
}
type LocalImage struct {
	ID         LocalID
	Key        NativeKey
	Platform   Platform
	containerd bool
}

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func unavailable() error { return imageDTO.Failure("update.check-unavailable", 503) }
func ParseLocalID(raw string) (LocalID, error) {
	if !digestPattern.MatchString(raw) {
		return LocalID{}, unavailable()
	}
	return LocalID{raw}, nil
}
func ParseExpected(raw string) (ExpectedLocalID, error) {
	if !digestPattern.MatchString(raw) {
		return ExpectedLocalID{}, unavailable()
	}
	return ExpectedLocalID{raw}, nil
}
func ParseTopLevel(raw string) (TopLevelDigest, error) {
	if !digestPattern.MatchString(raw) {
		return TopLevelDigest{}, unavailable()
	}
	return TopLevelDigest{raw}, nil
}
func ParsePlatform(raw string) (Platform, error) {
	parts := strings.Split(raw, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return Platform{}, unavailable()
	}
	for _, part := range parts {
		if part == "" || strings.ContainsAny(part, " \t\r\n:") {
			return Platform{}, unavailable()
		}
	}
	p := ocispec.Platform{OS: parts[0], Architecture: parts[1]}
	if len(parts) == 3 {
		p.Variant = parts[2]
	}
	switch p.Architecture {
	case "x86_64":
		p.Architecture = "amd64"
	case "aarch64":
		p.Architecture = "arm64"
	case "armv7l":
		p.Architecture = "arm"
		p.Variant = "v7"
	case "armv6l":
		p.Architecture = "arm"
		p.Variant = "v6"
	}
	return Platform{p}, nil
}
func NewCandidate(manifest, config string, p Platform) (RegistryCandidate, error) {
	if !digestPattern.MatchString(manifest) || !digestPattern.MatchString(config) || p.String() == "/" {
		return RegistryCandidate{}, unavailable()
	}
	return RegistryCandidate{RegistryManifest{manifest}, RegistryConfig{config}, p}, nil
}
func (v LocalID) String() string         { return v.value }
func (v NativeKey) String() string       { return v.value }
func (v ExpectedLocalID) String() string { return v.value }
func (v TopLevelDigest) String() string  { return v.value }
func (p Platform) String() string {
	s := p.value.OS + "/" + p.value.Architecture
	if p.value.Variant != "" {
		s += "/" + p.value.Variant
	}
	return s
}
func (p Platform) OCI() ocispec.Platform { return p.value }
func (p Platform) Registry() v1.Platform {
	return v1.Platform{OS: p.value.OS, Architecture: p.value.Architecture, Variant: p.value.Variant}
}
func MatchesExpected(local LocalID, expected ExpectedLocalID) bool {
	return local.value != "" && local.value == expected.value
}
func (l LocalImage) CandidateKey(r RegistryCandidate) (NativeKey, error) {
	if l.Platform.String() != r.platform.String() || l.Key.value == "" {
		return NativeKey{}, unavailable()
	}
	if l.containerd {
		return NativeKey{r.manifest.value}, nil
	}
	return NativeKey{r.config.value}, nil
}
func (l LocalImage) Expected(r RegistryCandidate) (ExpectedLocalID, error) {
	k, e := l.CandidateKey(r)
	return ExpectedLocalID{k.value}, e
}
func (l LocalImage) Digest() string {
	if l.containerd {
		return l.Key.value
	}
	return ""
}

// NativeObservation is the only durable provenance carrier for N/R comparison.
// The versioned marker lives in the existing platform column; legacy plain
// platforms cannot acquire provenance by merely becoming nonempty.
type NativeObservation struct {
	platform        Platform
	current, latest NativeKey
	containerd      bool
}

func (l LocalImage) Observe(r RegistryCandidate) (NativeObservation, error) {
	next, err := l.CandidateKey(r)
	return NativeObservation{l.Platform, l.Key, next, l.containerd}, err
}
func (n NativeObservation) Storage() (platform, current, latest string) {
	store := "classic"
	if n.containerd {
		store = "containerd"
	}
	return "native-v1:" + store + ":" + n.platform.String(), n.current.value, n.latest.value
}
func DecodeObservation(platform, current, latest string) (NativeObservation, error) {
	parts := strings.SplitN(platform, ":", 3)
	if len(parts) != 3 || parts[0] != "native-v1" || (parts[1] != "classic" && parts[1] != "containerd") || !digestPattern.MatchString(current) || !digestPattern.MatchString(latest) {
		return NativeObservation{}, unavailable()
	}
	p, err := ParsePlatform(parts[2])
	if err != nil {
		return NativeObservation{}, err
	}
	return NativeObservation{p, NativeKey{current}, NativeKey{latest}, parts[1] == "containerd"}, nil
}
func (n NativeObservation) Platform() string { return n.platform.String() }

func (l LocalImage) CurrentObservation() NativeObservation {
	return NativeObservation{l.Platform, l.Key, l.Key, l.containerd}
}
