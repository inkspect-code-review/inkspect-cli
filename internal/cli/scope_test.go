package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextScopeAndGeneralAsk(t *testing.T) {
	t.Chdir(t.TempDir())
	old := runGit
	defer func() { runGit = old }()
	remote := ""
	runGit = func(_ string, args ...string) ([]byte, error) {
		if strings.Join(args, " ") == "rev-parse --show-toplevel" {
			d, _ := os.Getwd()
			return []byte(d), nil
		}
		if strings.Join(args, " ") == "config --get remote.origin.url" && remote != "" {
			return []byte(remote), nil
		}
		return nil, errors.New("no checkout")
	}
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/cli/session" {
			json.NewEncoder(w).Encode(map[string]any{"workspace": "workspace", "repositories": []any{map[string]any{"fullName": "owner/repo"}}})
			return
		}
		if r.URL.Path != "/api/cli/context" {
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&body)
		json.NewEncoder(w).Encode(map[string]any{"sources": []any{}, "incomplete": true})
	}))
	defer server.Close()
	c := &Client{Config{server.URL, "test"}, server.Client()}
	for _, tc := range []struct{ name, remote, repo, all, want, selection string }{
		{name: "outside checkout", selection: "workspace"},
		{name: "detect SSH", remote: "git@github.com:Owner/Repo.git", want: "owner/repo", selection: "remote"},
		{name: "detect HTTPS", remote: "https://github.com/owner/repo.git", want: "owner/repo", selection: "remote"},
		{name: "unconnected remote", remote: "git@github.com:other/repo.git", selection: "workspace"},
		{name: "explicit", remote: "git@github.com:owner/repo.git", repo: "other/explicit", want: "other/explicit", selection: "explicit"},
		{name: "all bypasses checkout", remote: "git@github.com:owner/repo.git", all: "true", selection: "workspace"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote = tc.remote
			result, err := ask(c, "review engine", options{"repo": tc.repo, "all": tc.all})
			if err != nil {
				t.Fatal(err)
			}
			if body["repository"] != tc.want || result["selection"] != tc.selection {
				t.Fatalf("scope=%v selection=%v", body, result["selection"])
			}
			if result["freshness"] != "unknown" {
				t.Fatal("unverified scope reported current")
			}
		})
	}
	if _, err := ask(c, "query", options{"repo": "owner/repo", "all": "true"}); err == nil {
		t.Fatal("conflicting scope accepted")
	}
	os.WriteFile(".inkspect.json", []byte(`{"repository":"configured/repo","workspace":"pinned"}`), 0600)
	p, _, selection, err := contextScope(c, options{})
	if err != nil || p.Repository != "configured/repo" || p.Workspace != "pinned" || selection != "project" {
		t.Fatalf("project scope: %+v %s %v", p, selection, err)
	}
	p, _, _, err = contextScope(c, options{"all": "true"})
	if err != nil || p.Repository != "" || p.Workspace != "" {
		t.Fatal("--all retained project scope")
	}
}

func TestInitWithoutRepoAndAutomaticProject(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	old := runGit
	defer func() { runGit = old }()
	remote := ""
	runGit = func(_ string, args ...string) ([]byte, error) {
		if strings.Join(args, " ") == "rev-parse --show-toplevel" {
			return []byte(root), nil
		}
		if remote != "" && strings.Join(args, " ") == "config --get remote.origin.url" {
			return []byte(remote), nil
		}
		return nil, errors.New("not a checkout")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"workspace": "w", "repositories": []any{map[string]any{"fullName": "owner/repo"}}})
	}))
	defer server.Close()
	oldPath := configPath
	defer func() { configPath = oldPath }()
	configPath = func() (string, error) { return filepath.Join(root, "credentials.json"), nil }
	t.Setenv("INKSPECT_API_URL", server.URL)
	t.Setenv("INKSPECT_API_KEY", "isk_live_"+strings.Repeat("a", 64))
	var out bytes.Buffer
	if err := Run([]string{"init"}, strings.NewReader(""), &out, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(".inkspect.json"); !os.IsNotExist(err) {
		t.Fatal("general init created a project binding")
	}
	remote = "git@github.com:owner/repo.git"
	if err := Run([]string{"init"}, strings.NewReader(""), &out, &out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".inkspect.json")
	if err != nil || !bytes.Contains(data, []byte("owner/repo")) {
		t.Fatalf("auto init: %s %v", data, err)
	}
	if err := Run([]string{"init"}, strings.NewReader(""), &out, &out); err == nil {
		t.Fatal("existing binding overwritten")
	}
}

func TestGitHubRemoteParsing(t *testing.T) {
	for _, remote := range []string{"git@github.com:owner/repo.git", "https://github.com/owner/repo.git", "ssh://git@github.com/owner/repo.git"} {
		if githubRepository(remote) != "owner/repo" {
			t.Fatal(remote)
		}
	}
	for _, remote := range []string{"https://github.com.evil/owner/repo", "https://gitlab.com/owner/repo", "https://github.com/owner/repo?token=private", "/tmp/owner/repo", "git@github.com:owner/../repo"} {
		if githubRepository(remote) != "" {
			t.Fatal(remote)
		}
	}
}
