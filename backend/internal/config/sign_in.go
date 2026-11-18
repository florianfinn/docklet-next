// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original startup sign-in policy; M15 simple-user rights and identity contracts; M29 AUTH_MODE as forced override of the stored policy.

package config

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"

	"github.com/florianfinn/docklet-next/types/auth"
)

// SignInMode returns the forced mode from AUTH_MODE, or local when it is unset.
// The effective hub policy lives in the auth package; this is the env override only.
func (c *Config) SignInMode() auth.SignInMode {
	if mode, ok := c.ForcedSignInMode(); ok {
		return mode
	}
	return auth.SignInModeLocal
}

// ForcedSignInMode reports a non-empty AUTH_MODE, which fixes the mode and, for proxy, the proxy settings.
func (c *Config) ForcedSignInMode() (auth.SignInMode, bool) {
	if c == nil || strings.TrimSpace(c.AuthMode) == "" {
		return "", false
	}
	return auth.SignInMode(strings.TrimSpace(c.AuthMode)), true
}

func (c *Config) ProxyIdentityHeader() string {
	if c.AuthProxyHeader == "" {
		return "Remote-User"
	}
	return c.AuthProxyHeader
}

func (c *Config) ValidateSignIn() error {
	c.AppUrl = normalizedAppURL(c.AppUrl)
	if c.AgentMode || c.EdgeAgent {
		return nil
	}
	if c.AuthProxyLogoutURL != "" {
		u, err := url.Parse(c.AuthProxyLogoutURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("AUTH_PROXY_LOGOUT_URL must be an absolute http or https URL")
		}
	}
	mode, forced := c.ForcedSignInMode()
	if !forced {
		return nil
	}
	switch mode {
	case auth.SignInModeNone, auth.SignInModeLocal:
		return nil
	case auth.SignInModeProxy:
		if _, err := c.AuthProxyNetworks(); err != nil {
			return err
		}
		return ValidateProxyHeader(c.ProxyIdentityHeader())
	default:
		return errors.New("AUTH_MODE must be exactly one of none, local, proxy")
	}
}

// ValidateProxyHeader accepts an HTTP header field name (RFC 9110 token).
func ValidateProxyHeader(header string) error {
	if header == "" {
		return errors.New("AUTH_PROXY_HEADER must be an HTTP header name")
	}
	for _, ch := range header {
		if !strings.ContainsRune("!#$%&'*+-.^_`|~0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", ch) {
			return errors.New("AUTH_PROXY_HEADER must be an HTTP header name")
		}
	}
	return nil
}

func (c *Config) AuthProxyNetworks() ([]netip.Prefix, error) {
	return ParseTrustedProxies(c.AuthTrustedProxies)
}

// ParseTrustedProxies parses a comma-separated list of IP addresses or CIDRs; all-address networks are refused.
func ParseTrustedProxies(list string) ([]netip.Prefix, error) {
	if strings.TrimSpace(list) == "" {
		return nil, errors.New("AUTH_TRUSTED_PROXIES is required in proxy mode")
	}
	var networks []netip.Prefix
	for value := range strings.SplitSeq(list, ",") {
		value = strings.TrimSpace(value)
		network, err := netip.ParsePrefix(value)
		if err != nil {
			addr, addrErr := netip.ParseAddr(value)
			if addrErr != nil || addr.Zone() != "" {
				return nil, errors.New("AUTH_TRUSTED_PROXIES must contain only IP addresses or CIDRs")
			}
			network = netip.PrefixFrom(addr, addr.BitLen())
		}
		if network.Bits() == 0 {
			return nil, errors.New("AUTH_TRUSTED_PROXIES must not trust all addresses")
		}
		networks = append(networks, network.Masked())
	}
	return networks, nil
}
