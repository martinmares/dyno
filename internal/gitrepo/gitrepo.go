package gitrepo

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultPullInterval = 5 * time.Minute

// Config holds git repo parameters parsed from CLI + dyno-library.yaml.
type Config struct {
	RepoURL      string
	Branch       string        // "" → git default (usually main)
	PullInterval time.Duration // 0 → defaultPullInterval; <0 → disabled
	WorkDir      string        // writable directory where repo is cloned
}

// CloneOrUpdate ensures the repo is present in WorkDir.
// Returns the path to the cloned directory.
func CloneOrUpdate(cfg Config) (string, error) {
	if err := os.MkdirAll(cfg.WorkDir, 0o755); err != nil {
		return "", fmt.Errorf("work-dir: %w", err)
	}

	cloneDir := cloneDirFor(cfg)

	if _, err := os.Stat(filepath.Join(cloneDir, ".git")); os.IsNotExist(err) {
		slog.Info("cloning git repo", "url", cfg.RepoURL, "dest", cloneDir)
		args := []string{"clone", "--depth=1"}
		if cfg.Branch != "" {
			args = append(args, "--branch", cfg.Branch)
		}
		args = append(args, cfg.RepoURL, cloneDir)
		if out, err := git(args...); err != nil {
			return "", fmt.Errorf("git clone: %w\n%s", err, out)
		}
		slog.Info("cloned", "dest", cloneDir)
	}

	return cloneDir, nil
}

// StartAutoPull runs a background goroutine that periodically pulls the repo
// and calls onChange when new commits are detected. Stops when ctx is done.
func StartAutoPull(cfg Config, cloneDir string, onChange func()) {
	interval := cfg.PullInterval
	if interval == 0 {
		interval = defaultPullInterval
	}
	if interval < 0 {
		return // disabled
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			changed, err := pull(cfg, cloneDir)
			if err != nil {
				slog.Warn("git pull failed", "err", err)
				continue
			}
			if changed {
				slog.Info("git pull: new commits, reloading", "dir", cloneDir)
				onChange()
			}
		}
	}()
}

// pull runs git pull and returns true when new commits were fetched.
func pull(cfg Config, cloneDir string) (bool, error) {
	args := []string{"-C", cloneDir, "pull", "--ff-only"}
	if cfg.Branch != "" {
		args = append(args, "origin", cfg.Branch)
	}
	out, err := git(args...)
	if err != nil {
		return false, fmt.Errorf("%w\n%s", err, out)
	}
	changed := !strings.Contains(out, "Already up to date")
	return changed, nil
}

func git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// cloneDirFor returns a stable subdirectory name derived from the repo URL.
func cloneDirFor(cfg Config) string {
	name := filepath.Base(strings.TrimSuffix(cfg.RepoURL, ".git"))
	if name == "" || name == "." {
		name = "repo"
	}
	return filepath.Join(cfg.WorkDir, name)
}

// ParseInterval parses a pull interval string: "false"/"off"/"0" → disabled,
// otherwise a Go duration string (e.g. "5m", "1h").
func ParseInterval(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, nil // use default
	}
	if s == "false" || s == "off" || s == "0" || s == "disabled" {
		return -1, nil // disabled
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid git_pull_interval %q: %w", s, err)
	}
	if d < 30*time.Second {
		return 0, fmt.Errorf("git_pull_interval must be at least 30s, got %s", d)
	}
	return d, nil
}
