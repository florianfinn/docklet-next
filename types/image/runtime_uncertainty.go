// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

package image

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type RuntimeIdentityPeer struct {
	PeerID   string `json:"peerId" minLength:"1" maxLength:"256"`
	Name     string `json:"name" minLength:"1" maxLength:"128"`
	Reason   string `json:"reason" enum:"compose.identity-unreadable,compose.selection-unsupported,compose.env-file-missing"`
	Archived bool   `json:"archived"`
	File     string `json:"file,omitempty" maxLength:"255"`
}

type RuntimeUncertainty struct {
	Digest string                `json:"digest" pattern:"^[0-9a-f]{64}$"`
	Count  int                   `json:"count" minimum:"1" maximum:"9007199254740991"`
	Peers  []RuntimeIdentityPeer `json:"peers" minItems:"1" maxItems:"16"`
}

// RuntimePeerOverrides is durable evidence of the fresh resolution actually used.
// The count covers the full set; the snapshot has an independent storage bound.
type RuntimePeerOverrides struct {
	Count int                   `json:"count"`
	Peers []RuntimeIdentityPeer `json:"peers" maxItems:"64"`
}

func ValidRuntimeUncertaintyDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func RuntimePeerReason(value string) bool {
	switch value {
	case "compose.identity-unreadable", "compose.selection-unsupported", "compose.env-file-missing":
		return true
	}
	return false
}

func SafeRuntimePeerID(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func SafeRuntimeFile(value string) bool {
	if value == "" || len(value) > 255 || !utf8.ValidString(value) || strings.ContainsAny(value, "\\:") || strings.ContainsFunc(value, unicode.IsControl) {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func safeRuntimePeer(value RuntimeIdentityPeer) bool {
	return SafeRuntimePeerID(value.PeerID) && safeRuntimeName(value.Name) && RuntimePeerReason(value.Reason) && (value.File == "" || SafeRuntimeFile(value.File))
}

// NewRuntimeUncertainty hashes every ID/reason pair, independent of display order.
// Callers pass the complete fresh uncertainty set, never just a display page.
func NewRuntimeUncertainty(peers []RuntimeIdentityPeer) *RuntimeUncertainty {
	if len(peers) == 0 {
		return nil
	}
	keys := make([]string, len(peers))
	display := append([]RuntimeIdentityPeer(nil), peers...)
	for i, peer := range peers {
		keys[i] = peer.PeerID + ":" + peer.Reason
	}
	sort.Strings(keys)
	sum := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	sort.Slice(display, func(i, j int) bool {
		if display[i].Name == display[j].Name {
			return display[i].PeerID < display[j].PeerID
		}
		return display[i].Name < display[j].Name
	})
	if len(display) > 16 {
		display = display[:16]
	}
	return &RuntimeUncertainty{Digest: hex.EncodeToString(sum[:]), Count: len(peers), Peers: display}
}

func SafeRuntimePeerOverrides(value *RuntimePeerOverrides) *RuntimePeerOverrides {
	if value == nil || value.Count < 1 || value.Count > 9007199254740991 || len(value.Peers) != min(value.Count, 64) {
		return nil
	}
	seen := map[string]bool{}
	for _, peer := range value.Peers {
		if !safeRuntimePeer(peer) || seen[peer.PeerID] {
			return nil
		}
		seen[peer.PeerID] = true
	}
	copy := *value
	copy.Peers = append([]RuntimeIdentityPeer(nil), value.Peers...)
	return &copy
}
