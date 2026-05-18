package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// SiteConfig holds the user-defined site configuration from dyno.yaml.
type SiteConfig struct {
	Title        string  `yaml:"title"`
	Description  string  `yaml:"description"`
	Version      string  `yaml:"version"`
	LogoURL      string  `yaml:"logo_url"`
	LogoText     string  `yaml:"logo_text"`
	GitHubURL    string  `yaml:"github_url"`    // e.g. "https://github.com/org/repo"
	GitHubBranch string  `yaml:"github_branch"` // default: "main"
	Copyright    string  `yaml:"copyright"`
	Favicon      string  `yaml:"favicon"`   // URL or path under assets/
	BasePath     *string `yaml:"base_path"` // e.g. "/docs" (default), "" or "/" for root
	// Library mode fields (used when multiple --site flags are given)
	Slug                         string   `yaml:"slug"`  // URL segment for this book, e.g. "monitoring"
	Icon                         string   `yaml:"icon"`  // emoji or short text shown on library card
	Color                        string   `yaml:"color"` // accent hex color for library card, e.g. "#0ea5e9"
	APIProxyAllowedHosts         []string `yaml:"api_proxy_allowed_hosts"`
	APIProxyAllowPrivateNetworks *bool    `yaml:"api_proxy_allow_private_networks"`
	ContentInclude               []string `yaml:"content_include"`
	ContentExclude               []string `yaml:"content_exclude"`
	// Git auto-pull (used when site is cloned from a remote repo)
	GitPullInterval string `yaml:"git_pull_interval"` // e.g. "5m", "false" to disable
	GitBranch       string `yaml:"git_branch"`        // overrides CLI --git-branch
}

// GetSlug returns the URL slug for this site in library mode.
// Falls back to deriving a slug from dirName (e.g. "site-monitoring" → "monitoring").
func (c *SiteConfig) GetSlug(dirName string) string {
	if c.Slug != "" {
		return c.Slug
	}
	// Strip common prefixes: "site-", "docs-", "site_", "docs_"
	slug := dirName
	for _, prefix := range []string{"site-", "docs-", "site_", "docs_"} {
		if strings.HasPrefix(slug, prefix) {
			slug = strings.TrimPrefix(slug, prefix)
			break
		}
	}
	// Lowercase, replace spaces/underscores with hyphens
	slug = strings.ToLower(slug)
	slug = strings.ReplaceAll(slug, "_", "-")
	slug = strings.ReplaceAll(slug, " ", "-")
	if slug == "" {
		slug = dirName
	}
	return slug
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

// MergeDefaults fills in missing fields from ext (external source, e.g. dyno-library.yaml entry).
// Only fields that are still at their zero value are overwritten — dyno.yaml values always win.
func (c *SiteConfig) MergeDefaults(ext SiteConfig) {
	if c.Title == "" || c.Title == "Docs" {
		if ext.Title != "" {
			c.Title = ext.Title
		}
	}
	if c.Description == "" && ext.Description != "" {
		c.Description = ext.Description
	}
	if c.LogoText == "" || c.LogoText == c.Title {
		if ext.LogoText != "" {
			c.LogoText = ext.LogoText
		}
	}
	if c.Slug == "" && ext.Slug != "" {
		c.Slug = ext.Slug
	}
	if c.Icon == "" && ext.Icon != "" {
		c.Icon = ext.Icon
	}
	if c.Color == "" && ext.Color != "" {
		c.Color = ext.Color
	}
	if c.GitHubURL == "" && ext.GitHubURL != "" {
		c.GitHubURL = ext.GitHubURL
	}
	if c.GitHubBranch == "" || c.GitHubBranch == "main" {
		if ext.GitHubBranch != "" {
			c.GitHubBranch = ext.GitHubBranch
		}
	}
	if c.Copyright == "" && ext.Copyright != "" {
		c.Copyright = ext.Copyright
	}
	if c.BasePath == nil && ext.BasePath != nil {
		c.BasePath = ext.BasePath
	}
	if c.GitPullInterval == "" && ext.GitPullInterval != "" {
		c.GitPullInterval = ext.GitPullInterval
	}
	if c.GitBranch == "" && ext.GitBranch != "" {
		c.GitBranch = ext.GitBranch
	}
	if len(c.ContentInclude) == 0 && len(ext.ContentInclude) > 0 {
		c.ContentInclude = ext.ContentInclude
	}
	if len(c.ContentExclude) == 0 && len(ext.ContentExclude) > 0 {
		c.ContentExclude = ext.ContentExclude
	}
}

// Exists reports whether a dyno.yaml file exists in dir.
func Exists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "dyno.yaml"))
	return err == nil
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

// LoadForContent reads dyno.yaml for a site. For direct content directories it
// prefers contentDir/dyno.yaml and falls back to rootDir/dyno.yaml.
func LoadForContent(rootDir, contentDir string) (*SiteConfig, error) {
	cfgDir := rootDir
	if Exists(contentDir) {
		cfgDir = contentDir
	}
	return Load(cfgDir)
}

// LoadTemplateEnv loads optional siteRoot/.env values and overlays process env.
// Process env wins, so container/pod configuration overrides file values.
func LoadTemplateEnv(siteRoot string) (map[string]string, error) {
	return LoadTemplateEnvForContent(siteRoot, filepath.Join(siteRoot, "site"))
}

// LoadTemplateEnvForContent loads optional .env values from rootDir and
// contentDir and overlays process env. Process env wins, so container/pod
// configuration overrides file values.
func LoadTemplateEnvForContent(rootDir, contentDir string) (map[string]string, error) {
	values := make(map[string]string)

	envPaths := []string{
		filepath.Join(rootDir, ".env"),
	}
	if contentDir != "" {
		contentEnv := filepath.Join(contentDir, ".env")
		if contentEnv != envPaths[0] {
			envPaths = append(envPaths, contentEnv)
		}
	}
	for _, path := range envPaths {
		if err := loadEnvFile(path, values); err != nil {
			return nil, err
		}
	}

	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}

	return values, nil
}

func loadEnvFile(path string, values map[string]string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		values[key] = value
	}
	return scanner.Err()
}
