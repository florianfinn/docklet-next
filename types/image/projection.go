// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229; #634 build times and lent observations.

package image

import (
	"strconv"
	"strings"
	"time"
)

// ProjectUpdate compares store-native keys at one verified platform: config
// digests under classic, platform manifests under containerd. Callers prove roles.
func ProjectUpdate(info *UpdateInfo) {
	info.CheckedAt = info.CheckTime
	if info.AuthState == "" {
		info.AuthState = "unknown"
		switch info.AuthMethod {
		case "anonymous", "credential", "authentication-required", "authentication-failed":
			info.AuthState = info.AuthMethod
		}
		if info.UsedCredential {
			info.AuthState = "credential"
		}
	}
	info.AuthUsername = ""
	info.UsedCredential = info.AuthState == "credential"
	if info.AuthState == "credential" || info.AuthState == "anonymous" {
		info.AuthMethod = info.AuthState
	}
	if info.Error != "" {
		info.State = "error"
		info.HasUpdate = false
		if info.Code == "" {
			info.Code = "update.check-unavailable"
		}
		info.Error = "The update check could not be completed."
		return
	}
	if info.CheckTime.IsZero() {
		info.State = "unchecked"
		info.HasUpdate = false
		return
	}
	if info.UpdateType == "local" {
		info.State = "local"
		info.HasUpdate = false
		return
	}
	if strings.Contains(info.ImageRef, "@sha256:") {
		info.State = "current"
		info.HasUpdate = false
		return
	}
	if info.Platform == "" || info.CurrentDigest == "" || info.LatestDigest == "" {
		info.State = "unknown"
		info.HasUpdate = false
		return
	}
	info.State = "current"
	info.HasUpdate = false
	if info.CurrentDigest != info.LatestDigest {
		info.State = "rebuild"
		info.HasUpdate = true
	}
	current := "v" + strings.TrimPrefix(info.CurrentVersion, "v")
	candidate := "v" + strings.TrimPrefix(info.LatestVersion, "v")
	if compared, valid := CompareRelease(candidate, current); valid && compared > 0 && info.CurrentDigest != info.LatestDigest {
		info.State = "version"
		info.HasUpdate = true
		a := strings.Split(strings.TrimPrefix(info.CurrentVersion, "v"), ".")
		b := strings.Split(strings.TrimPrefix(info.LatestVersion, "v"), ".")
		info.UpdateType = "patch"
		if a[0] != b[0] {
			info.UpdateType = "major"
		} else if a[1] != b[1] {
			info.UpdateType = "minor"
		}
	}
}

// CompareRelease accepts stable numeric SemVer releases; prerelease/ambiguous
// labels never become a version candidate through lexical or time ordering.
func CompareRelease(a, b string) (int, bool) {
	parse := func(raw string) ([3]uint64, bool) {
		var values [3]uint64
		raw = strings.TrimPrefix(raw, "v")
		raw, _, _ = strings.Cut(raw, "+")
		parts := strings.Split(raw, ".")
		if len(parts) != 3 {
			return values, false
		}
		for i, p := range parts {
			if p == "" || (len(p) > 1 && p[0] == '0') {
				return values, false
			}
			for _, ch := range p {
				if ch < '0' || ch > '9' {
					return values, false
				}
			}
			v, e := strconv.ParseUint(p, 10, 64)
			if e != nil {
				return values, false
			}
			values[i] = v
		}
		return values, true
	}
	av, ok := parse(a)
	if !ok {
		return 0, false
	}
	bv, ok := parse(b)
	if !ok {
		return 0, false
	}
	for i := range av {
		if av[i] > bv[i] {
			return 1, true
		}
		if av[i] < bv[i] {
			return -1, true
		}
	}
	return 0, true
}

// BuildTime keeps a plausible image config build time; reproducible builds
// often stamp the epoch, which names no build. The UTC value must stay
// encodable as RFC 3339 (year 9999 at most).
func BuildTime(t time.Time) *time.Time {
	u := t.UTC()
	if u.Year() < 2000 || u.Year() > 9999 {
		return nil
	}
	return &u
}

// BuildTimeText reads a Docker inspect Created value through BuildTime.
func BuildTimeText(s string) *time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil
	}
	return BuildTime(t)
}

// Lend copies an observation for a consumer it was not made for. Build facts
// and the platform that vouches for them stay with the observing consumer.
func Lend(info *UpdateInfo) *UpdateInfo {
	if info == nil {
		return nil
	}
	out := *info
	out.Platform, out.CurrentCreated, out.LatestCreated = "", nil, nil
	return &out
}
