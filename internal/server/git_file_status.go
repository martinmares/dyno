package server

import (
	"os/exec"
	"path/filepath"
	"strings"
)

func gitFileStatus(filePath string) string {
	gitRoot := findGitRoot(filepath.Dir(filePath))
	if gitRoot == "" {
		return ""
	}
	rel, err := filepath.Rel(gitRoot, filePath)
	if err != nil {
		return ""
	}
	out, err := exec.Command("git", "-C", gitRoot, "status", "--porcelain=v1", "--untracked-files=all", "--", rel).Output()
	if err != nil {
		return ""
	}
	line := strings.TrimRight(string(out), "\r\n")
	if len(line) < 2 {
		return ""
	}
	return parseGitFileStatus(line[:2])
}

func parseGitFileStatus(code string) string {
	if code == "??" {
		return "Untracked"
	}
	if len(code) != 2 {
		return ""
	}
	staged := code[0] != ' '
	modified := code[1] != ' '
	switch {
	case staged && modified:
		return "Staged + modified"
	case staged:
		return "Staged"
	case modified:
		return "Modified"
	default:
		return ""
	}
}
