package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const help = `Inkspect — Cloud context and reviews

  login [--no-browser | --token-stdin] [--api-url https://app.inkspect.si]
  whoami [--format json|markdown]
  logout
  init [--repo owner/name]
  ask [question | -] [--repo owner/name | --all] [--format json|markdown]
      [--budget 12000] [--explain] [--require-current]
  index [--repo owner/name] [--no-wait] [--timeout 1800] [--format json|markdown]
  index status [--repo owner/name] [--format json|markdown]
  source <source-id> [--around 40]
  status | doctor
  update                       Update the native CLI after checksum verification
  review local [--staged | --base main] [--include-untracked] [--dry-run]
      [--task "Review these changes"] [--no-wait]
  review pr <number> [--repo owner/name] [--no-wait]
  review rerun <review-id> [--repo owner/name] [--no-wait]
  review status|show <review-id> [--repo owner/name]
  skill install|update|updates|list|uninstall [--agent codex] [--scope project|global]
  mcp serve

Set INKSPECT_API_KEY for automation. Context and reviews use your Cloud credits.
Local review uploads selected source to Cloud; --dry-run uploads nothing.
No engine or language runtime is required. Git is needed for local snapshots.
`

type options map[string]string

var booleanFlags = map[string]bool{"all": true, "explain": true, "require-current": true, "staged": true, "include-untracked": true, "dry-run": true, "no-wait": true, "token-stdin": true, "no-browser": true, "help": true, "version": true}
var valueFlags = map[string]bool{"repo": true, "format": true, "budget": true, "around": true, "base": true, "task": true, "api-url": true, "agent": true, "scope": true, "timeout": true}

func parse(args []string) ([]string, options, error) {
	pos := []string{}
	o := options{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "--") {
			pos = append(pos, a)
			continue
		}
		k, v, has := strings.Cut(a[2:], "=")
		if _, ok := o[k]; ok {
			return nil, nil, fmt.Errorf("duplicate option --%s", k)
		}
		if booleanFlags[k] {
			if has {
				return nil, nil, fmt.Errorf("--%s does not take a value", k)
			}
			o[k] = "true"
		} else if valueFlags[k] {
			if !has {
				i++
				if i >= len(args) {
					return nil, nil, fmt.Errorf("missing value for --%s", k)
				}
				v = args[i]
			}
			o[k] = v
		} else {
			return nil, nil, fmt.Errorf("unknown option --%s", k)
		}
	}
	if f := o["format"]; f != "" && f != "json" && f != "markdown" {
		return nil, nil, errors.New("format must be json or markdown")
	}
	return pos, o, nil
}
func integer(o options, key string, def, min, max int) (int, error) {
	v := def
	var e error
	if s, ok := o[key]; ok {
		v, e = strconv.Atoi(s)
	}
	if e != nil || v < min || v > max {
		return 0, fmt.Errorf("--%s must be between %d and %d", key, min, max)
	}
	return v, nil
}
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	s, e := f.Stat()
	return e == nil && s.Mode()&os.ModeCharDevice != 0
}
func readInput(r io.Reader, limit int64) (string, error) {
	b, e := io.ReadAll(io.LimitReader(r, limit+1))
	if e != nil {
		return "", e
	}
	if int64(len(b)) > limit {
		return "", errors.New("input exceeds size limit")
	}
	return strings.TrimSpace(string(b)), nil
}
func repoFor(o options) (Project, string, error) {
	p, root, e := project()
	if e != nil {
		return p, root, e
	}
	if o["repo"] != "" {
		p.Repository = o["repo"]
		p.Workspace = ""
	}
	if p.Repository == "" {
		return p, root, errors.New("run inkspect init --repo owner/name or pass --repo")
	}
	return p, root, nil
}

func Run(args []string, in io.Reader, out, log io.Writer) error {
	pos, o, e := parse(args)
	if e != nil {
		return e
	}
	if o["version"] != "" {
		fmt.Fprintln(out, Version)
		return nil
	}
	if len(pos) == 0 || o["help"] != "" {
		fmt.Fprint(out, help)
		return nil
	}
	if e = validateOptions(pos, o); e != nil {
		return e
	}
	if pos[0] != "mcp" && pos[0] != "update" && !(pos[0] == "skill" && len(pos) > 1 && pos[1] == "updates") && o["format"] != "json" && isTerminal(in) {
		if file, ok := out.(*os.File); ok && isTerminal(file) {
			notifyUpdates(log)
		}
	}
	if pos[0] == "update" {
		return updateCLI(out)
	}
	if pos[0] == "skill" {
		return skillCommand(pos[1:], o, out)
	}
	if pos[0] == "logout" {
		p, e := configPath()
		if e != nil {
			return e
		}
		if e = os.Remove(p); e != nil && !os.IsNotExist(e) {
			return e
		}
		fmt.Fprintln(out, "Saved credentials removed. Environment credentials remain active; revoke the key in Cloud to disable it everywhere.")
		return nil
	}
	if pos[0] == "login" {
		if o["token-stdin"] != "" && o["no-browser"] != "" {
			return errors.New("choose --token-stdin or --no-browser")
		}
		if o["token-stdin"] == "" {
			return browserLogin(o, in, out, log)
		}
		token, e := readInput(in, 256)
		if e != nil {
			return e
		}
		if !strings.HasPrefix(token, "isk_live_") || len(token) != 73 {
			return errors.New("invalid API key")
		}
		origin, e := loginOrigin(o)
		if e != nil {
			return e
		}
		c := &Client{Config{origin, token}, httpClient()}
		if _, e = c.request("GET", "/api/cli/session", nil); e != nil {
			return e
		}
		if e = saveConfig(c.Config); e != nil {
			return e
		}
		fmt.Fprintln(out, "Signed in. Credentials saved in your private user configuration.")
		return nil
	}
	if len(pos) == 2 && pos[0] == "review" && pos[1] == "local" && o["dry-run"] != "" {
		return reviewCommand(nil, pos[1:], o, out, log)
	}
	c, e := newClient()
	if e != nil {
		return e
	}
	if pos[0] == "mcp" && len(pos) == 2 && pos[1] == "serve" {
		return serveMCP(c, in, out, log)
	}
	switch pos[0] {
	case "init":
		p, root, _, e := contextScope(c, o)
		if e != nil {
			return e
		}
		v, e := c.request("GET", "/api/cli/session", nil)
		if e != nil {
			return e
		}
		if p.Repository == "" {
			fmt.Fprintf(out, "Connected to workspace %v. Ask from any directory to search accessible repositories; no project configuration is needed.\n", v["workspace"])
			return nil
		}
		found := false
		for _, r := range array(v["repositories"]) {
			m, _ := r.(map[string]any)
			if name, ok := m["fullName"].(string); ok && strings.EqualFold(name, p.Repository) {
				p.Repository = name
				found = true
			}
		}
		if !found {
			return errors.New("repository is not available to this CLI key")
		}
		p.Workspace = fmt.Sprint(v["workspace"])
		b, _ := json.MarshalIndent(p, "", "  ")
		f, e := os.OpenFile(filepath.Join(root, ".inkspect.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return errors.New("could not create .inkspect.json; an existing configuration is never overwritten")
		}
		_, e = f.Write(append(b, '\n'))
		ce := f.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
		fmt.Fprintln(out, "Connected", p.Repository)
		return nil
	case "whoami":
		return whoami(c, o, out)
	case "status", "doctor":
		v, e := c.request("GET", "/api/cli/session", nil)
		if e != nil {
			return e
		}
		p, root, e := project()
		if e != nil {
			return e
		}
		v["project"] = p
		v["local"] = freshness(root)
		v["cli_version"] = Version
		if p.Repository != "" {
			state, err := c.request("GET", "/api/cli/status?repository="+url.QueryEscape(p.Repository)+"&workspace="+url.QueryEscape(p.Workspace), nil)
			if err != nil {
				return err
			}
			v["cloud"] = state
		}
		return output(out, v, o["format"])
	case "ask":
		if len(pos) == 1 {
			if !isTerminal(in) {
				return errors.New("noninteractive ask requires a question or '-' to read stdin")
			}
			return interactive(c, o, in, out, log)
		}
		q := strings.Join(pos[1:], " ")
		if q == "-" {
			q, e = readInput(in, 6000)
			if e != nil {
				return e
			}
		}
		v, e := askWithProgress(c, q, o, in, out, log)
		if e != nil {
			return e
		}
		return output(out, v, o["format"])
	case "index":
		return indexCommand(c, pos[1:], o, in, out, log)
	case "source":
		if len(pos) != 2 {
			return errors.New("source requires one source ID")
		}
		around, e := integer(o, "around", 40, 0, 150)
		if e != nil {
			return e
		}
		v, e := c.request("POST", "/api/cli/source", map[string]any{"id": pos[1], "around": around})
		if e != nil {
			return e
		}
		return output(out, v, o["format"])
	case "review":
		return reviewCommand(c, pos[1:], o, out, log)
	default:
		return errors.New("unknown command; run inkspect --help")
	}
}
func array(v any) []any { a, _ := v.([]any); return a }
func validateOptions(pos []string, o options) error {
	command := pos[0]
	if (command == "review" || command == "skill" || command == "mcp" || command == "index") && len(pos) > 1 {
		command += " " + pos[1]
	}
	allowed := map[string]string{
		"login": "token-stdin no-browser api-url", "whoami": "format", "logout": "", "init": "repo", "status": "format", "doctor": "format", "update": "",
		"index": "repo format no-wait timeout", "index status": "repo format",
		"ask": "repo all format budget explain require-current", "source": "format around", "mcp serve": "",
		"review local": "format staged base include-untracked dry-run task no-wait timeout",
		"review pr":    "repo format no-wait timeout", "review rerun": "repo format no-wait timeout",
		"review status": "repo format", "review show": "repo format",
		"skill install": "agent scope", "skill update": "agent scope", "skill updates": "agent scope format", "skill list": "agent scope format", "skill uninstall": "agent scope",
	}
	keys, known := allowed[command]
	if !known {
		return errors.New("unknown command; run inkspect --help")
	}
	for k := range o {
		if !strings.Contains(" "+keys+" ", " "+k+" ") {
			return fmt.Errorf("--%s is not supported by %s", k, command)
		}
	}
	if command != "ask" && command != "review pr" && command != "review rerun" && command != "review status" && command != "review show" && command != "source" {
		expected := 1
		if strings.Contains(command, " ") {
			expected = 2
		}
		if len(pos) != expected {
			return errors.New("unexpected command arguments")
		}
	}
	return nil
}
func ask(c *Client, q string, o options) (map[string]any, error) {
	return askContext(context.Background(), c, q, o)
}
func askContext(ctx context.Context, c *Client, q string, o options) (map[string]any, error) {
	if strings.TrimSpace(q) == "" || len(q) > 6000 {
		return nil, errors.New("question must contain 1–6000 UTF-8 bytes")
	}
	p, root, selection, e := contextScopeContext(ctx, c, o)
	if e != nil {
		return nil, e
	}
	budget, e := integer(o, "budget", 12000, 256, 64000)
	if e != nil {
		return nil, e
	}
	v, e := c.requestContext(ctx, "POST", "/api/cli/context", map[string]any{"question": q, "repository": p.Repository, "workspace": p.Workspace, "budget": budget, "explain": o["explain"] != ""})
	if e != nil {
		return nil, e
	}
	local := freshness(root)
	state := "unknown"
	if p.Repository != "" && local["repository"] == p.Repository && local["revision"] != "" {
		state = "current"
		if local["dirty"] != false {
			state = "local_changes"
		}
		if len(array(v["sources"])) == 0 {
			state = "unknown"
		}
		for _, raw := range array(v["sources"]) {
			s, _ := raw.(map[string]any)
			if s["revision"] != local["revision"] {
				state = "stale"
			}
		}
	}
	v["selection"] = selection
	v["freshness"] = state
	v["local"] = local
	if o["require-current"] != "" && (state != "current" || v["incomplete"] == true) {
		return nil, fmt.Errorf("current complete context required; freshness=%s", state)
	}
	return v, nil
}
func interactive(c *Client, o options, in io.Reader, out, log io.Writer) error {
	fmt.Fprintln(log, "Inkspect context. Follow up to refine; /clear resets, /exit quits.")
	s := bufio.NewScanner(in)
	s.Buffer(make([]byte, 1024), 6001)
	history := ""
	for {
		fmt.Fprint(log, "> ")
		if !s.Scan() {
			return s.Err()
		}
		q := strings.TrimSpace(s.Text())
		if q == "/exit" {
			return nil
		}
		if q == "/clear" {
			history = ""
			continue
		}
		if q == "" {
			continue
		}
		query := q
		if history != "" {
			query = history + "\nFollow-up: " + q
		}
		if len(query) > 6000 {
			fmt.Fprintln(log, "Question history is full. Use /clear.")
			continue
		}
		v, e := askWithProgress(c, query, o, in, out, log)
		if e != nil {
			fmt.Fprintln(log, e)
			continue
		}
		history = query
		if e = output(out, v, o["format"]); e != nil {
			return e
		}
	}
}
func reviewCommand(c *Client, pos []string, o options, out, log io.Writer) error {
	if len(pos) == 0 {
		return errors.New("choose review local, pr, rerun, status or show")
	}
	var v map[string]any
	var e error
	localSnapshotID := ""
	timeout, e := integer(o, "timeout", 900, 1, 3600)
	if e != nil {
		return e
	}
	switch pos[0] {
	case "local":
		snap, e := collectSnapshot(o)
		if e != nil {
			return e
		}
		if o["dry-run"] != "" {
			paths := []string{}
			for _, f := range snap.Files {
				paths = append(paths, f.Path)
			}
			return output(out, map[string]any{"files": paths, "omissions": snap.Omissions, "snapshot_id": snap.SnapshotID, "uploads": false}, o["format"])
		}
		fmt.Fprintln(log, "Uploading selected source snapshot to Inkspect Cloud for review.")
		localSnapshotID = snap.SnapshotID
		v, e = c.request("POST", "/api/cli/reviews/local", snap)
	case "pr", "rerun", "status", "show":
		if len(pos) != 2 {
			return errors.New("review operation requires a PR number or review ID")
		}
		p, _, pe := repoFor(o)
		if pe != nil && !(len(pos[1]) == 36 && (pos[0] == "status" || pos[0] == "show")) {
			return pe
		}
		body := map[string]any{"repository": p.Repository, "workspace": p.Workspace}
		if pos[0] == "pr" {
			n, pe := strconv.Atoi(pos[1])
			if pe != nil || n < 1 {
				return errors.New("invalid PR number")
			}
			body["number"] = n
		} else {
			body["id"] = pos[1]
		}
		if pos[0] == "show" || pos[0] == "status" {
			v, e = c.request("GET", "/api/cli/reviews/"+pos[0]+"?repository="+url.QueryEscape(p.Repository)+"&id="+url.QueryEscape(pos[1]), nil)
		} else {
			v, e = c.request("POST", "/api/cli/reviews/"+pos[0], body)
		}
	default:
		return errors.New("unknown review operation")
	}
	if e != nil {
		return e
	}
	if pos[0] == "show" || pos[0] == "status" || o["no-wait"] != "" {
		return output(out, v, o["format"])
	}
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	if pendingReview(v) {
		fmt.Fprintln(log, "Review", reviewID(v), "is running. Interrupting the wait does not cancel the Cloud job.")
	}
	for pendingReview(v) {
		if time.Now().After(deadline) {
			_ = output(out, v, o["format"])
			return errors.New("review is still running; use review status with its ID")
		}
		time.Sleep(2 * time.Second)
		id := reviewID(v)
		p, _, _ := project()
		if o["repo"] != "" {
			p.Repository = o["repo"]
		}
		v, e = c.request("GET", "/api/cli/reviews/show?id="+url.QueryEscape(id)+"&repository="+url.QueryEscape(p.Repository), nil)
		if e != nil {
			return e
		}
	}
	if localSnapshotID != "" {
		v["local_freshness"] = "unknown"
		if current, err := collectSnapshot(o); err == nil {
			if current.SnapshotID == localSnapshotID {
				v["local_freshness"] = "current"
			} else {
				v["local_freshness"] = "changed_since_submission"
			}
		}
	}
	if e = output(out, v, o["format"]); e != nil {
		return e
	}
	if reviewState(v) == "failed" || reviewState(v) == "cancelled" {
		return errors.New("review did not complete successfully")
	}
	return nil
}
func reviewID(v map[string]any) string {
	if x, ok := v["id"].(string); ok {
		return x
	}
	j, _ := v["job"].(map[string]any)
	return fmt.Sprint(j["id"])
}
func reviewState(v map[string]any) string {
	if x, ok := v["status"].(string); ok {
		return x
	}
	j, _ := v["job"].(map[string]any)
	return fmt.Sprint(j["status"])
}
func pendingReview(v map[string]any) bool {
	s := reviewState(v)
	return s == "queued" || s == "running" || s == "pending" || s == "paused"
}
