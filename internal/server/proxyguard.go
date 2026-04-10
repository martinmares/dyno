package server

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

func (s *Server) validateProxyTarget(ctx context.Context, rawURL string) error {
	target, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return fmt.Errorf("only http and https targets are allowed")
	}
	if target.User != nil {
		return fmt.Errorf("credentials in target url are not allowed")
	}

	host := strings.TrimSpace(target.Hostname())
	if host == "" {
		return fmt.Errorf("missing target host")
	}

	if len(s.siteCfg.APIProxyAllowedHosts) > 0 {
		allowed := false
		for _, candidate := range s.siteCfg.APIProxyAllowedHosts {
			if strings.EqualFold(strings.TrimSpace(candidate), host) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("target host %q is not in api_proxy_allowed_hosts", host)
		}
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("failed to resolve target host: %w", err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("target host resolved to no addresses")
	}
	for _, addr := range addrs {
		if blockedProxyIP(addr.IP, s.siteCfg.AllowPrivateProxyTargets()) {
			return fmt.Errorf("target host resolves to blocked address %s", addr.IP.String())
		}
	}

	return nil
}

func blockedProxyIP(ip net.IP, allowPrivate bool) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	if addr.IsUnspecified() || addr.IsMulticast() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return true
	}
	if allowPrivate {
		return false
	}
	return addr.IsLoopback() || addr.IsPrivate()
}
