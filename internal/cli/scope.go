package cli

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var repositoryName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_.-]+$`)

func githubRepository(remote string) string {
	remote = strings.TrimSpace(remote)
	path := ""
	if strings.HasPrefix(remote, "git@github.com:") {
		path = strings.TrimPrefix(remote, "git@github.com:")
	} else if u, err := url.Parse(remote); err == nil && strings.EqualFold(u.Hostname(), "github.com") && (u.Scheme == "https" || u.Scheme == "ssh") && u.RawQuery == "" && u.Fragment == "" {
		path = strings.TrimPrefix(u.Path, "/")
	}
	path = strings.TrimSuffix(strings.TrimSuffix(path, "/"), ".git")
	if !repositoryName.MatchString(path) || strings.HasSuffix(path, "/.") || strings.HasSuffix(path, "/..") {
		return ""
	}
	return path
}

func detectedProject(root string) (Project, string) {
	if top, err := runGit(root, "rev-parse", "--show-toplevel"); err == nil {
		root = strings.TrimSpace(string(top))
	}
	remote, err := runGit(root, "config", "--get", "remote.origin.url")
	if err != nil {
		return Project{}, root
	}
	return Project{Repository: githubRepository(string(remote))}, root
}

// Explicit scope never silently widens. Automatic remotes are checked against
// the signed-in workspace before becoming a repository filter.
func contextScope(c *Client, o options) (Project, string, string, error) {
	return contextScopeContext(context.Background(), c, o)
}
func contextScopeContext(ctx context.Context, c *Client, o options) (Project, string, string, error) {
	if o["all"] != "" && o["repo"] != "" {
		return Project{}, "", "", errors.New("choose --repo or --all")
	}
	p, root, err := project()
	if o["all"] != "" {
		return Project{}, root, "workspace", nil
	}
	if o["repo"] != "" {
		return Project{Repository: o["repo"]}, root, "explicit", nil
	}
	if err != nil || p.Repository != "" {
		return p, root, "project", err
	}
	p, root = detectedProject(root)
	if p.Repository == "" {
		return p, root, "workspace", nil
	}
	session, err := c.requestContext(ctx, "GET", "/api/cli/session", nil)
	if err != nil {
		return Project{}, root, "", err
	}
	for _, raw := range array(session["repositories"]) {
		r, _ := raw.(map[string]any)
		name, _ := r["fullName"].(string)
		if strings.EqualFold(name, p.Repository) {
			p.Repository = name
			p.Workspace, _ = session["workspace"].(string)
			return p, root, "remote", nil
		}
	}
	return Project{}, root, "workspace", nil
}
