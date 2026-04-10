package server

import (
	"context"
	"testing"

	"github.com/mares/dyno/internal/config"
)

func TestValidateProxyTargetBlocksPrivateWhenDisabled(t *testing.T) {
	allowPrivate := false
	s := &Server{
		siteCfg:   &config.SiteConfig{APIProxyAllowPrivateNetworks: &allowPrivate},
		metrics:   newMetrics(),
		pageCache: map[string]pageCacheEntry{},
	}

	if err := s.validateProxyTarget(context.Background(), "http://127.0.0.1:8080"); err == nil {
		t.Fatal("expected loopback target to be blocked")
	}
}

func TestValidateProxyTargetHonorsAllowlist(t *testing.T) {
	allowPrivate := true
	s := &Server{
		siteCfg: &config.SiteConfig{
			APIProxyAllowedHosts:         []string{"example.com"},
			APIProxyAllowPrivateNetworks: &allowPrivate,
		},
		metrics:   newMetrics(),
		pageCache: map[string]pageCacheEntry{},
	}

	if err := s.validateProxyTarget(context.Background(), "https://httpbin.org/get"); err == nil {
		t.Fatal("expected host outside allowlist to be blocked")
	}
}
