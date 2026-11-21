package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var Version = "0.1.2"

const DefaultOrigin = "https://app.inkspect.si"

type Config struct {
	Origin string `json:"origin"`
	Token  string `json:"token,omitempty"`
}
type Project struct {
	Repository string `json:"repository"`
	Workspace  string `json:"workspace"`
}
type Client struct {
	Config Config
	HTTP   *http.Client
}

var configPath = func() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "inkspect", "credentials.json"), nil
}

func loadConfig() (Config, error) {
	c := Config{Origin: DefaultOrigin}
	p, e := configPath()
	if e != nil {
		return c, e
	}
	b, e := os.ReadFile(p)
	if e == nil {
		if json.Unmarshal(b, &c) != nil {
			return c, errors.New("invalid saved configuration")
		}
	} else if !os.IsNotExist(e) {
		return c, e
	}
	if v := os.Getenv("INKSPECT_API_KEY"); v != "" {
		c.Token = v
	}
	if v := os.Getenv("INKSPECT_API_URL"); v != "" {
		c.Origin = v
	}
	return c, validateOrigin(c.Origin)
}
func validateOrigin(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("API URL must be an origin without credentials, path or query")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")) {
		return errors.New("API URL must use HTTPS")
	}
	return nil
}
func secureDir(dir string) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	info, e := os.Lstat(dir)
	if e != nil {
		return e
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("configuration directory must not be a symlink")
	}
	if runtime.GOOS == "windows" {
		u, e := user.Current()
		if e != nil {
			return e
		}
		if e = exec.Command("icacls", dir, "/inheritance:r", "/grant:r", "*"+u.Uid+":(OI)(CI)F").Run(); e != nil {
			return errors.New("could not restrict credential directory permissions")
		}
		return nil
	}
	return os.Chmod(dir, 0700)
}
func saveConfig(c Config) error {
	p, e := configPath()
	if e != nil {
		return e
	}
	if e = secureDir(filepath.Dir(p)); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p), "credentials-*.tmp")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	b, e := json.Marshal(c)
	if e != nil {
		f.Close()
		return e
	}
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	// Windows cannot atomically rename over an existing file; remove only our credential file.
	if runtime.GOOS == "windows" {
		if e = os.Remove(p); e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	return os.Rename(f.Name(), p)
}
func newClient() (*Client, error) {
	c, e := loadConfig()
	if e != nil {
		return nil, e
	}
	if c.Token == "" {
		return nil, errors.New("sign in with inkspect login or set INKSPECT_API_KEY")
	}
	return &Client{c, &http.Client{Timeout: 190 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) request(method, path string, body any) (map[string]any, error) {
	return c.requestContext(context.Background(), method, path, body)
}
func (c *Client) requestContext(ctx context.Context, method, path string, body any) (map[string]any, error) {
	var r io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return nil, e
		}
		r = bytes.NewReader(b)
	}
	req, e := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Config.Origin, "/")+path, r)
	if e != nil {
		return nil, errors.New("invalid API request")
	}
	req.Header.Set("Authorization", "Bearer "+c.Config.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "inkspect-cli/"+Version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, e := c.HTTP.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("Cloud request failed; check connectivity or run inkspect doctor (no automatic retry)")
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, 2_000_001))
	if e != nil || len(b) > 2_000_000 {
		return nil, errors.New("Cloud response exceeded the limit")
	}
	var v map[string]any
	if json.Unmarshal(b, &v) != nil {
		return nil, fmt.Errorf("Cloud returned HTTP %d without valid JSON", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := v["error"].(string)
		if len(msg) > 500 {
			msg = msg[:500]
		}
		msg = strings.ReplaceAll(msg, c.Config.Token, "[redacted]")
		return nil, fmt.Errorf("Cloud HTTP %d: %s", resp.StatusCode, terminalText(msg))
	}
	return v, nil
}
func project() (Project, string, error) {
	cwd, e := os.Getwd()
	if e != nil {
		return Project{}, "", e
	}
	for d := cwd; ; d = filepath.Dir(d) {
		b, e := os.ReadFile(filepath.Join(d, ".inkspect.json"))
		if e == nil {
			var p Project
			if json.Unmarshal(b, &p) != nil {
				return p, d, errors.New("invalid .inkspect.json")
			}
			return p, d, nil
		}
		if !os.IsNotExist(e) {
			return Project{}, d, e
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	return Project{}, cwd, nil
}
func terminalText(s string) string {
	return strings.Map(func(r rune) rune {
		if r == 27 || (r < 32 && r != '\n' && r != '\t') {
			return -1
		}
		return r
	}, s)
}
func output(w io.Writer, v any, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return output(w, v, "json")
	}
	if sources, ok := m["sources"].([]any); ok {
		fmt.Fprintf(w, "# Inkspect context\n\nFreshness: %v · incomplete: %v\n", m["freshness"], m["incomplete"])
		if scope, ok := m["scope"].(map[string]any); ok {
			fmt.Fprintf(w, "Scope: %s", terminalText(fmt.Sprint(scope["kind"])))
			if repo, ok := scope["repository"].(string); ok {
				fmt.Fprintf(w, " · %s", terminalText(repo))
			}
			if count, ok := scope["searched_repositories"]; ok {
				fmt.Fprintf(w, " · searched %v/%v repositories", count, scope["accessible_repositories"])
			}
			fmt.Fprintln(w)
		}
		for _, raw := range sources {
			s, _ := raw.(map[string]any)
			fmt.Fprintf(w, "\n## %s · %s:%v–%v\nRevision: %s\nSource: %s\n\n", terminalText(fmt.Sprint(s["repository"])), terminalText(fmt.Sprint(s["path"])), s["startLine"], s["endLine"], s["revision"], s["id"])
			content := terminalText(fmt.Sprint(s["content"]))
			fence := "```"
			for strings.Contains(content, fence) {
				fence += "`"
			}
			fmt.Fprintf(w, "%s\n%s\n%s\n", fence, content, fence)
		}
		if len(sources) == 0 {
			fmt.Fprintln(w, "\nNo matching source was returned.")
		}
		if omissions := array(m["omissions"]); len(omissions) > 0 {
			fmt.Fprintln(w, "\nOmissions:")
			for _, item := range omissions {
				fmt.Fprintln(w, "-", terminalText(fmt.Sprint(item)))
			}
		}
		if x, ok := m["explanation"]; ok {
			fmt.Fprintln(w, "\nSelection details:")
			return output(w, x, "json")
		}
		return nil
	}
	if content, ok := m["content"].(string); ok {
		fmt.Fprintln(w, terminalText(content))
		return nil
	}
	if raw, ok := m["report"].(map[string]any); ok {
		report := raw
		if inner, ok := raw["review"].(map[string]any); ok {
			report = inner
		}
		fmt.Fprintf(w, "# Inkspect review %s\n\nStatus: %s\n\n", reviewID(m), reviewState(m))
		if state, ok := m["local_freshness"]; ok {
			fmt.Fprintf(w, "Local snapshot: %v\n\n", state)
		}
		if presentation, ok := report["presentation"].(map[string]any); ok {
			if markdown, ok := presentation["markdown"].(string); ok {
				fmt.Fprintln(w, terminalText(markdown))
				return nil
			}
		}
		if summary, ok := report["summary"].(string); ok {
			fmt.Fprintln(w, terminalText(summary))
		}
		for _, item := range array(report["findings"]) {
			finding, ok := item.(map[string]any)
			if !ok {
				continue
			}
			fmt.Fprintf(w, "\n## %s: %s\n", terminalText(fmt.Sprint(finding["severity"])), terminalText(fmt.Sprint(finding["title"])))
			if location, ok := finding["location"].(map[string]any); ok {
				fmt.Fprintf(w, "%s:%v\n", terminalText(fmt.Sprint(location["path"])), location["start_line"])
			}
			for _, field := range []string{"reason", "suggestion", "verification"} {
				if text, ok := finding[field].(string); ok {
					fmt.Fprintln(w, terminalText(text))
				}
			}
		}
		return nil
	}
	return output(w, v, "json")
}
