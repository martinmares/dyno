package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func releaseFixture(t *testing.T) (string, string) {
	t.Helper()
	for _, tool := range []string{"bash", "git", "curl", "jq", "shasum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("release script tests need %s", tool)
		}
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ci_release_common.sh", "ci_release_notes.sh", "ci_publish_release.sh", "ci_publish_github_release.sh"} {
		data, err := os.ReadFile(filepath.Join("scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		releasePut(t, filepath.Join(dir, "scripts", name), string(data))
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=CI Test", "-c", "user.email=ci@example.com"}, args...)...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	git("init", "--quiet")
	releasePut(t, filepath.Join(dir, "VERSION"), "1.2.2\n")
	git("add", "VERSION")
	git("commit", "--quiet", "-m", "Baseline")
	git("tag", "v1.2.2")
	releasePut(t, filepath.Join(dir, "VERSION"), "1.2.3\n")
	git("add", "VERSION")
	git("commit", "--quiet", "-m", "Add release feature")
	commit := git("rev-parse", "HEAD")
	if output, err := runReleaseScript(dir, "ci_release_notes.sh", commit); err != nil {
		t.Fatalf("notes: %v: %s", err, output)
	}
	archive := "dyno-1.2.3-linux-amd64.tar.gz"
	contents := "test package"
	releasePut(t, filepath.Join(dir, "dist", "release", archive), contents)
	releasePut(t, filepath.Join(dir, "dist", "release", "SHA256SUMS"), fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte(contents)), archive))
	return dir, commit
}

func releasePut(t *testing.T, name, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}

func runReleaseScript(dir, script, commit string, env ...string) ([]byte, error) {
	cmd := exec.Command("bash", "scripts/"+script)
	cmd.Dir = dir
	// Do not inherit release metadata from the enclosing CI job.
	for _, item := range os.Environ() {
		key := strings.SplitN(item, "=", 2)[0]
		if !strings.HasPrefix(key, "RELEASE_") && key != "GITHUB_SHA" && key != "CI_COMMIT_SHA" {
			cmd.Env = append(cmd.Env, item)
		}
	}
	cmd.Env = append(cmd.Env, "RELEASE_COMMIT="+commit)
	cmd.Env = append(cmd.Env, env...)
	return cmd.CombinedOutput()
}

func TestReleaseNotesAndVersionValidation(t *testing.T) {
	dir, commit := releaseFixture(t)
	notes, err := os.ReadFile(filepath.Join(dir, "dist", "release_notes.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(notes), "Changes since v1.2.2") || !strings.Contains(string(notes), "Add release feature") || strings.Contains(string(notes), "Baseline") {
		t.Fatalf("wrong release notes: %s", notes)
	}
	if output, err := runReleaseScript(dir, "ci_release_notes.sh", commit, "RELEASE_VERSION=9.9.9"); err == nil {
		t.Fatalf("accepted mismatched version: %s", output)
	}
	releasePut(t, filepath.Join(dir, "VERSION"), "../unsafe\n")
	if output, err := runReleaseScript(dir, "ci_release_notes.sh", commit); err == nil {
		t.Fatalf("accepted unsafe version: %s", output)
	}
}

func TestGitLabReleasePublicationAndRetry(t *testing.T) {
	dir, commit := releaseFixture(t)
	exists, uploads, updates := false, 0, 0
	var mu sync.Mutex
	var links []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("JOB-TOKEN") != "test-token" {
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimPrefix(r.URL.Path, "/projects/7")
		switch {
		case r.Method == "GET" && path == "/repository/tags/v1.2.3":
			if !exists {
				http.NotFound(w, r)
				return
			}
			fmt.Fprintf(w, `{"commit":{"id":%q}}`, commit)
		case r.Method == "GET" && path == "/releases/v1.2.3":
			if !exists {
				http.NotFound(w, r)
				return
			}
			fmt.Fprintf(w, `{"commit":{"id":%q}}`, commit)
		case r.Method == "GET" && path == "/releases/v1.2.3/assets/links":
			_ = json.NewEncoder(w).Encode(links)
		case r.Method == "PUT" && strings.HasPrefix(path, "/packages/generic/dyno/1.2.3/"):
			uploads++
			fmt.Fprint(w, `{}`)
		case r.Method == "POST" && path == "/releases":
			var payload struct {
				Ref    string `json:"ref"`
				Assets struct {
					Links []map[string]any `json:"links"`
				} `json:"assets"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Ref != commit || len(payload.Assets.Links) != 2 {
				http.Error(w, "bad release payload", 400)
				return
			}
			links = payload.Assets.Links
			for i := range links {
				links[i]["id"] = i + 1
			}
			exists = true
			fmt.Fprint(w, `{}`)
		case r.Method == "PUT" && (path == "/releases/v1.2.3" || strings.HasPrefix(path, "/releases/v1.2.3/assets/links/")):
			updates++
			fmt.Fprint(w, `{}`)
		default:
			http.Error(w, "unexpected request: "+r.Method+" "+path, 400)
		}
	}))
	defer server.Close()
	for i := 0; i < 2; i++ {
		output, err := runReleaseScript(dir, "ci_publish_release.sh", commit, "CI_API_V4_URL="+server.URL, "CI_PROJECT_ID=7", "CI_JOB_TOKEN=test-token")
		if err != nil {
			t.Fatalf("publication %d: %v: %s", i, err, output)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if !exists || uploads != 4 || updates != 3 || len(links) != 2 {
		t.Fatalf("wrong publication/retry: exists=%v uploads=%d updates=%d links=%d", exists, uploads, updates, len(links))
	}
}

func TestGitLabPublicationFailsOnAuthenticationError(t *testing.T) {
	dir, commit := releaseFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", 403)
	}))
	defer server.Close()
	output, err := runReleaseScript(dir, "ci_publish_release.sh", commit, "CI_API_V4_URL="+server.URL, "CI_PROJECT_ID=7", "CI_JOB_TOKEN=test-token")
	if err == nil || !strings.Contains(string(output), "HTTP 403") {
		t.Fatalf("authentication error was ignored: %v: %s", err, output)
	}
}

func TestGitLabPublicationRejectsDifferentTagCommit(t *testing.T) {
	dir, commit := releaseFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/repository/tags/") {
			http.Error(w, "publication must not proceed", 500)
			return
		}
		fmt.Fprintf(w, `{"commit":{"id":%q}}`, strings.Repeat("0", 40))
	}))
	defer server.Close()
	output, err := runReleaseScript(dir, "ci_publish_release.sh", commit, "CI_API_V4_URL="+server.URL, "CI_PROJECT_ID=7", "CI_JOB_TOKEN=test-token")
	if err == nil || !strings.Contains(string(output), "different commit") {
		t.Fatalf("replaced existing tag: %v: %s", err, output)
	}
}

func TestGitHubReleasePublication(t *testing.T) {
	dir, commit := releaseFixture(t)
	tool := filepath.Join(dir, "tools", "gh")
	releasePut(t, tool, `#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$1 $2" in
  'release view') test "${GH_EXISTING:-0}" = 1 ;;
  'api repos/'*) printf '%s\n' "$RELEASE_COMMIT" ;;
  'release create'|'release upload'|'release edit') exit 0 ;;
  *) exit 2 ;;
esac
`)
	if err := os.Chmod(tool, 0755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "gh-calls")
	env := []string{
		"PATH=" + filepath.Dir(tool) + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GH_TOKEN=test-token", "GH_REPO=test/dyno", "GH_CALL_LOG=" + log,
	}
	for _, existing := range []string{"0", "1"} {
		output, err := runReleaseScript(dir, "ci_publish_github_release.sh", commit, append(env, "GH_EXISTING="+existing)...)
		if err != nil {
			t.Fatalf("GitHub publication: %v: %s", err, output)
		}
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"release create v1.2.3", "--draft\n", "release upload v1.2.3", "--clobber", "release edit v1.2.3", "--draft=false", "--prerelease=false"} {
		if !strings.Contains(string(calls), want) {
			t.Fatalf("missing %q in GitHub calls: %s", want, calls)
		}
	}
	if strings.Index(string(calls), "release edit") < strings.Index(string(calls), "release upload") {
		t.Fatal("GitHub release published before uploads")
	}
	if strings.Count(string(calls), "release create") != 1 || strings.Count(string(calls), "release upload") != 2 {
		t.Fatalf("GitHub retry created a duplicate release: %s", calls)
	}
}
