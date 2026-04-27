package libraryconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SiteEntry describes one site in dyno-library.yaml.
// Exactly one of URL or Path must be set.
type SiteEntry struct {
	// Git repo source
	URL          string `yaml:"url"`
	Branch       string `yaml:"branch"`
	PullInterval string `yaml:"pull_interval"`
	// Local directory source
	Path string `yaml:"path"`
	// Metadata (fallback if dyno.yaml absent in the site)
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	LogoText    string `yaml:"logo_text"`
	Slug        string `yaml:"slug"`
	Icon        string `yaml:"icon"`
	Color       string `yaml:"color"`
	GitHubURL   string `yaml:"github_url"`
	BasePath    string `yaml:"base_path"`
}

// IsGit reports whether this entry is a git repo.
func (e SiteEntry) IsGit() bool { return e.URL != "" }

// LibraryFile is the top-level structure of dyno-library.yaml.
type LibraryFile struct {
	Title    string      `yaml:"title"`
	LogoText string      `yaml:"logo_text"`
	BasePath string      `yaml:"base_path"`
	WorkDir  string      `yaml:"work_dir"` // for git clones; CLI --work-dir takes precedence
	Sites    []SiteEntry `yaml:"sites"`
}

// Load parses a dyno-library.yaml file.
// Relative paths inside the file are resolved relative to the file's directory.
func Load(path string) (*LibraryFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var lf LibraryFile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	// Resolve relative paths relative to the library file's directory.
	base := filepath.Dir(filepath.Clean(path))
	for i, s := range lf.Sites {
		if s.Path != "" && !filepath.IsAbs(s.Path) {
			lf.Sites[i].Path = filepath.Join(base, s.Path)
		}
	}
	if lf.WorkDir != "" && !filepath.IsAbs(lf.WorkDir) {
		lf.WorkDir = filepath.Join(base, lf.WorkDir)
	}

	// Validate entries.
	for i, s := range lf.Sites {
		if s.URL == "" && s.Path == "" {
			return nil, fmt.Errorf("sites[%d]: must set either 'url' or 'path'", i)
		}
		if s.URL != "" && s.Path != "" {
			return nil, fmt.Errorf("sites[%d]: cannot set both 'url' and 'path'", i)
		}
	}

	return &lf, nil
}
