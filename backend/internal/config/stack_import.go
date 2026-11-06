// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package config

import (
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

const DefaultStackImportGitHubRawURL = "https://raw.githubusercontent.com"

// StackImportBaseURL validates the operator-owned mirror before the resolver is wired.
// Plain HTTP requires a loopback hostname or a literal loopback/private address.
func StackImportBaseURL(raw string) (*url.URL, error) {
	if raw == "" {
		raw = DefaultStackImportGitHubRawURL
	}
	u, err := url.Parse(raw)
	fail := func() (*url.URL, error) {
		return nil, errors.New("STACK_IMPORT_GITHUB_RAW_URL must be an HTTPS base URL or an HTTP loopback/private address without credentials, query or fragment")
	}
	if err != nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return fail()
	}
	if p := u.Port(); p != "" {
		if n, e := strconv.Atoi(p); e != nil || n < 1 || n > 65535 {
			return fail()
		}
	}
	if u.Scheme != "https" {
		ip, e := netip.ParseAddr(u.Hostname())
		if u.Scheme != "http" || (u.Hostname() != "localhost" && (e != nil || ip.Zone() != "" || (!ip.Unmap().IsLoopback() && !ip.Unmap().IsPrivate()))) {
			return fail()
		}
	}
	for _, s := range strings.Split(u.Path, "/") {
		if s == ".." || s == "." || strings.Contains(s, "\\") {
			return fail()
		}
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u, nil
}
