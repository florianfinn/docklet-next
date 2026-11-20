// Source: docklet-next/issues/465
// Revision: b4f894ff7f35186d4c39be1fc9fccd01b4b5934a
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original bounded runtime identity blocker detail.

package image

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// RuntimeIdentityBlocker identifies a peer without forwarding filesystem diagnostics.
type RuntimeIdentityBlocker struct {
	Name        string              `json:"name" minLength:"1" maxLength:"128"`
	Reason      string              `json:"reason" enum:"compose.selection-unsupported,compose.identity-unreadable,compose.env-file-missing,compose.name-undeterminable,compose.duplicate"`
	PeerID      string              `json:"peerId,omitempty"`
	Confirmable bool                `json:"confirmable,omitzero"`
	File        string              `json:"file,omitempty"`
	Uncertainty *RuntimeUncertainty `json:"uncertainty,omitempty"`
	Archived    bool                `json:"archived"`
}

// SafeRuntimeIdentityBlocker is the sole remote detail allowlist for this refusal.
func SafeRuntimeIdentityBlocker(code string, value *RuntimeIdentityBlocker) *RuntimeIdentityBlocker {
	if code != "runtime.target-unavailable" || value == nil || !safeRuntimeName(value.Name) {
		return nil
	}
	switch value.Reason {
	case "compose.selection-unsupported", "compose.identity-unreadable", "compose.env-file-missing", "compose.name-undeterminable", "compose.duplicate":
	default:
		return nil
	}
	copy := *value
	copy.Confirmable, copy.Uncertainty = false, nil
	valid := true
	if copy.PeerID != "" && !SafeRuntimePeerID(copy.PeerID) {
		copy.PeerID = ""
		valid = false
	}
	if copy.File != "" && !SafeRuntimeFile(copy.File) {
		copy.File = ""
		valid = false
	}
	if !valid {
		return &copy
	}
	u := value.Uncertainty
	if !value.Confirmable || value.Reason != "compose.name-undeterminable" || u == nil || !ValidRuntimeUncertaintyDigest(u.Digest) || u.Count < 1 || u.Count > 9007199254740991 || len(u.Peers) != min(u.Count, 16) {
		return &copy
	}
	seen := map[string]bool{}
	for _, peer := range u.Peers {
		if !safeRuntimePeer(peer) || seen[peer.PeerID] {
			return &copy
		}
		seen[peer.PeerID] = true
	}
	owned := *u
	owned.Digest = strings.ToLower(owned.Digest)
	owned.Peers = append([]RuntimeIdentityPeer(nil), u.Peers...)
	copy.Confirmable, copy.Uncertainty = true, &owned
	return &copy
}

// RuntimeDisplayName maps a user-entered stack name to a value safeRuntimeName
// accepts: disallowed runes become spaces, runs collapse, and the result is cut
// to 128 bytes. An empty result falls back to the peer ID.
func RuntimeDisplayName(name, peerID string) string {
	if safeRuntimeName(name) {
		return name
	}
	var b strings.Builder
	space := false
	for _, r := range strings.ToValidUTF8(name, " ") {
		if r == '/' || r == '\\' || r == ':' || unicode.IsControl(r) || unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			if b.Len()+1+utf8.RuneLen(r) > 128 {
				break
			}
			b.WriteByte(' ')
		}
		space = false
		if b.Len()+utf8.RuneLen(r) > 128 {
			break
		}
		b.WriteRune(r)
	}
	if out := b.String(); safeRuntimeName(out) {
		return out
	}
	return peerID
}

func safeRuntimeName(name string) bool {
	if !utf8.ValidString(name) || len(name) > 128 || strings.TrimSpace(name) == "" || strings.ContainsAny(name, "/\\:") {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) || (unicode.IsSpace(r) && r != ' ') {
			return false
		}
	}
	return true
}
