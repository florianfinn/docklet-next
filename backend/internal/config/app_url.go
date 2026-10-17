// Source: docklet-next/issues/68
// Revision: af5fa9df01e730f7691d650985abd33d74f51ae5
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original browser-compatible APP_URL scheme, host and port normalization.

package config

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

func normalizedAppURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" {
		return value
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if address, err := netip.ParseAddr(host); err == nil {
		host = address.String()
		if address.Is4In6() {
			// Browser IPv6 hosts keep hexadecimal words for mapped addresses.
			mapped := address.As4()
			host = fmt.Sprintf("::ffff:%x:%x", uint16(mapped[0])<<8|uint16(mapped[1]), uint16(mapped[2])<<8|uint16(mapped[3]))
		}
	}
	port := u.Port()
	if port != "" {
		if number, err := strconv.ParseUint(port, 10, 16); err == nil {
			port = strconv.FormatUint(number, 10)
		}
	}
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	} else {
		u.Host = host
	}
	return u.String()
}
