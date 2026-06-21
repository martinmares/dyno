package server

import "testing"

func TestParseGitFileStatus(t *testing.T) {
	tests := map[string]string{
		" M": "Modified",
		"M ": "Staged",
		"MM": "Staged + modified",
		"??": "Untracked",
		"  ": "",
		"":   "",
	}
	for code, want := range tests {
		if got := parseGitFileStatus(code); got != want {
			t.Errorf("parseGitFileStatus(%q) = %q, want %q", code, got, want)
		}
	}
}
