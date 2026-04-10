package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SiteConfig holds the user-defined site configuration from dyno.yaml.
type SiteConfig struct {
	Title                        string   `yaml:"title"`
	Description                  string   `yaml:"description"`
	Version                      string   `yaml:"version"`
	LogoURL                      string   `yaml:"logo_url"`
	LogoText                     string   `yaml:"logo_text"`
	GitHubURL                    string   `yaml:"github_url"`    // e.g. "https://github.com/org/repo"
	GitHubBranch                 string   `yaml:"github_branch"` // default: "main"
	Copyright                    string   `yaml:"copyright"`
	Favicon                      string   `yaml:"favicon"`   // URL or path under assets/
	BasePath                     *string  `yaml:"base_path"` // e.g. "/docs" (default), "" or "/" for root
	APIProxyAllowedHosts         []string `yaml:"api_proxy_allowed_hosts"`
	APIProxyAllowPrivateNetworks *bool    `yaml:"api_proxy_allow_private_networks"`
}

// GetBasePath returns the normalized base path as a string.
func (c *SiteConfig) GetBasePath() string {
	if c.BasePath == nil {
		return "/docs"
	}
	return *c.BasePath
}

// Defaults fills in sensible defaults for any missing fields.
func (c *SiteConfig) Defaults() {
	if c.Title == "" {
		c.Title = "Docs"
	}
	if c.LogoText == "" {
		c.LogoText = c.Title
	}
	if c.GitHubBranch == "" {
		c.GitHubBranch = "main"
	}
	if c.Copyright == "" {
		c.Copyright = c.Title
	}
	// BasePath: nil means not set → default "/docs"
	// "" or "/" means root (no prefix)
	if c.BasePath != nil {
		trimmed := strings.Trim(*c.BasePath, "/")
		var normalized string
		if trimmed != "" {
			normalized = "/" + trimmed
		}
		c.BasePath = &normalized
	}
	if c.APIProxyAllowPrivateNetworks == nil {
		allow := true
		c.APIProxyAllowPrivateNetworks = &allow
	}
}

func (c *SiteConfig) AllowPrivateProxyTargets() bool {
	return c.APIProxyAllowPrivateNetworks != nil && *c.APIProxyAllowPrivateNetworks
}

// Load reads dyno.yaml from siteRoot. If the file doesn't exist, returns defaults.
func Load(siteRoot string) (*SiteConfig, error) {
	cfg := &SiteConfig{}
	path := filepath.Join(siteRoot, "dyno.yaml")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg.Defaults()
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	cfg.Defaults()
	return cfg, nil
}
