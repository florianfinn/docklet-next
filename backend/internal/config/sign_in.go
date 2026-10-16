// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original startup sign-in policy.

package config

import (
	"errors"
	"net/netip"
	"strings"

	"github.com/florianfinn/docklet-next/types/auth"
)

func (c *Config) SignInMode() auth.SignInMode {
	if c == nil || c.AuthMode == "" {
		return auth.SignInModeLocal
	}
	return auth.SignInMode(c.AuthMode)
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
	switch c.SignInMode() {
	case auth.SignInModeNone, auth.SignInModeLocal:
		return nil
	case auth.SignInModeProxy:
		if _, err := c.AuthProxyNetworks(); err != nil {
			return err
		}
		for _, ch := range c.ProxyIdentityHeader() {
			if !strings.ContainsRune("!#$%&'*+-.^_`|~0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", ch) {
				return errors.New("AUTH_PROXY_HEADER must be an HTTP header name")
			}
		}
		return nil
	default:
		return errors.New("AUTH_MODE must be exactly one of none, local, proxy")
	}
}

func (c *Config) AuthProxyNetworks() ([]netip.Prefix, error) {
	if strings.TrimSpace(c.AuthTrustedProxies) == "" {
		return nil, errors.New("AUTH_TRUSTED_PROXIES is required in proxy mode")
	}
	var networks []netip.Prefix
	for value := range strings.SplitSeq(c.AuthTrustedProxies, ",") {
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
