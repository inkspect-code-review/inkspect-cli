package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type SnapshotFile struct {
	Path   string  `json:"path"`
	Before *string `json:"before"`
	After  *string `json:"after"`
	Role   string  `json:"role"`
}
type Omission struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
type Snapshot struct {
	SchemaVersion int               `json:"schema_version"`
	Task          map[string]any    `json:"task"`
	Baseline      map[string]string `json:"baseline"`
	SnapshotID    string            `json:"snapshot_id"`
	Files         []SnapshotFile    `json:"files"`
	Tests         []any             `json:"tests"`
	Omissions     []Omission        `json:"omissions"`
}

var runGit = func(root string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-pager", "--literal-pathspecs", "-c", "core.fsmonitor=false", "-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	var b boundedBuffer
	cmd.Stdout = &b
	cmd.Stderr = &bytes.Buffer{}
	if e := cmd.Run(); e != nil {
		return nil, errors.New("Git read failed; ensure Git is installed and the revision exists locally")
	}
	return b.Bytes(), nil
}

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 2_000_000 {
		return 0, errors.New("Git output exceeds limit")
	}
	return b.Buffer.Write(p)
}
func freshness(root string) map[string]any {
	v := map[string]any{"revision": "", "dirty": true, "repository": "", "verified": false}
	head, e := runGit(root, "rev-parse", "--verify", "HEAD")
	if e != nil {
		return v
	}
	v["revision"] = strings.TrimSpace(string(head))
	status, e := runGit(root, "status", "--porcelain", "--untracked-files=normal")
	if e != nil {
		return v
	}
	v["dirty"] = len(status) > 0
	remote, e := runGit(root, "config", "--get", "remote.origin.url")
	if e == nil {
		if name := githubRepository(string(remote)); name != "" {
			v["repository"] = name
			v["verified"] = true
		}
	}
	return v
}

var unsafePart = regexp.MustCompile(`(?i)^(?:\.git|\.ssh|\.aws|\.env(?:\..*)?|node_modules|vendor|target|dist|build|\.next|\.venv|__pycache__|credentials(?:\..*)?|secrets?(?:\..*)?)$`)
var unsafeExt = regexp.MustCompile(`(?i)\.(?:pem|key|p12|pfx|keystore|sqlite|db|log|tgz|zip)$`)
var credential = regexp.MustCompile(`-----BEGIN[^\n]*PRIVATE KEY-----|\b(?:isk_live_[a-f0-9]{64}|github_pat_[A-Za-z0-9_]{20,}|gh[pousr]_[A-Za-z0-9_]{30,}|sk-(?:proj-|ant-)?[A-Za-z0-9_-]{24,}|AKIA[A-Z0-9]{16})\b|(?:https?|postgres(?:ql)?):\/\/[^\s/:@]+:[^\s/@]+@`)

func safePath(p string) bool {
	if len(p) > 512 || p == "" || unsafeExt.MatchString(p) {
		return false
	}
	for _, r := range p {
		if r < 32 || r == 127 || r == '\\' || r == ':' || r > 0xffff {
			return false
		}
	}
	for _, s := range strings.Split(p, "/") {
		if s == "" || s == "." || s == ".." || unsafePart.MatchString(s) {
			return false
		}
	}
	return true
}
func encodeCanonical(v any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	s := strings.TrimSuffix(b.String(), "\n")
	s = strings.ReplaceAll(strings.ReplaceAll(s, `\u2028`, "\u2028"), `\u2029`, "\u2029")
	return []byte(s), nil
}
func digest(files []SnapshotFile) (string, error) {
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	b, e := encodeCanonical(files)
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), e
}

type ignoreRule struct {
	include bool
	re      *regexp.Regexp
}

func ignoreRules(b []byte) ([]ignoreRule, error) {
	if len(b) > 32768 || !utf8.Valid(b) || bytes.ContainsRune(b, 0) {
		return nil, errors.New("invalid .inkspectignore")
	}
	lines := strings.Split(strings.TrimSuffix(strings.TrimPrefix(string(b), "\ufeff"), "\n"), "\n")
	if len(lines) > 1025 {
		return nil, errors.New("too many ignore lines")
	}
	rules := []ignoreRule{}
	for _, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		include := strings.HasPrefix(s, "!")
		if include {
			s = s[1:]
		}
		if strings.HasPrefix(s, `\#`) || strings.HasPrefix(s, `\!`) {
			s = s[1:]
		}
		if len(s) > 512 || s == "" || strings.ContainsAny(s, "\\[]\x00") {
			return nil, errors.New("unsupported .inkspectignore syntax")
		}
		anchored := strings.HasPrefix(s, "/") || strings.Contains(strings.TrimSuffix(s, "/"), "/")
		directory := strings.HasSuffix(s, "/")
		s = strings.Trim(s, "/")
		if s == "" {
			return nil, errors.New("invalid ignore pattern")
		}
		parts := strings.Split(s, "/")
		pattern := ""
		for i, p := range parts {
			if p == "" || p == "." || p == ".." || strings.Contains(p, "**") && p != "**" {
				return nil, errors.New("invalid ignore segment")
			}
			if p == "**" {
				if i == len(parts)-1 {
					pattern += ".*"
				} else {
					pattern += "(?:[^/]+/)*"
				}
				continue
			}
			for _, r := range p {
				switch r {
				case '*':
					pattern += "[^/]*"
				case '?':
					pattern += "[^/]"
				default:
					pattern += regexp.QuoteMeta(string(r))
				}
			}
			if i < len(parts)-1 {
				pattern += "/"
			}
		}
		prefix := "(?:^|/)"
		if anchored {
			prefix = "^"
		}
		suffix := "(?:/.*)?$"
		if directory {
			suffix = "/.*$"
		}
		re, e := regexp.Compile(prefix + pattern + suffix)
		if e != nil {
			return nil, e
		}
		rules = append(rules, ignoreRule{include, re})
		if len(rules) > 512 {
			return nil, errors.New("too many ignore rules")
		}
	}
	return rules, nil
}
func excluded(p string, rules []ignoreRule) bool {
	if p == ".inkspectignore" {
		return false
	}
	out := false
	for _, r := range rules {
		if r.re.MatchString(p) {
			out = !r.include
		}
	}
	return out
}
func readTree(root, ref, p string) (*string, error) {
	b, e := runGit(root, "ls-tree", "-z", ref, "--", p)
	if e != nil {
		return nil, e
	}
	if len(b) == 0 {
		return nil, nil
	}
	fields := strings.Fields(strings.SplitN(string(b), "\t", 2)[0])
	if len(fields) < 3 || fields[0] != "100644" && fields[0] != "100755" {
		return nil, errors.New("symlink or non-file")
	}
	b, e = runGit(root, "cat-file", "blob", fields[2])
	if e != nil {
		return nil, e
	}
	return textSource(b)
}
func textSource(b []byte) (*string, error) {
	if len(b) > 64000 || !utf8.Valid(b) || bytes.ContainsRune(b, 0) {
		return nil, errors.New("binary, non-UTF-8 or oversized file")
	}
	s := string(b)
	if credential.MatchString(s) {
		return nil, errors.New("possible credential")
	}
	return &s, nil
}
func readWorking(root, p string) (*string, error) {
	current := root
	for _, part := range strings.Split(p, "/") {
		current = filepath.Join(current, part)
		info, e := os.Lstat(current)
		if os.IsNotExist(e) {
			return nil, nil
		}
		if e != nil {
			return nil, e
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("symlink")
		}
	}
	info, e := os.Stat(current)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() || info.Size() > 64000 {
		return nil, errors.New("non-regular or oversized file")
	}
	b, e := os.ReadFile(current)
	if e != nil {
		return nil, e
	}
	return textSource(b)
}
func readStaged(root, p string) (*string, error) {
	b, e := runGit(root, "ls-files", "--stage", "-z", "--", p)
	if e != nil {
		return nil, e
	}
	if len(b) == 0 {
		return nil, nil
	}
	if bytes.Count(b, []byte{0}) != 1 {
		return nil, errors.New("unmerged file")
	}
	f := strings.Fields(strings.SplitN(string(b), "\t", 2)[0])
	if len(f) != 3 || f[2] != "0" || f[0] != "100644" && f[0] != "100755" {
		return nil, errors.New("symlink or unmerged file")
	}
	b, e = runGit(root, "cat-file", "blob", f[1])
	if e != nil {
		return nil, e
	}
	return textSource(b)
}
func collectSnapshot(o options) (Snapshot, error) {
	snap := Snapshot{SchemaVersion: 1, Files: []SnapshotFile{}, Tests: []any{}, Omissions: []Omission{}}
	if o["staged"] != "" && (o["base"] != "" || o["include-untracked"] != "") {
		return snap, errors.New("--staged cannot be combined with --base or --include-untracked")
	}
	cwd, e := os.Getwd()
	if e != nil {
		return snap, e
	}
	b, e := runGit(cwd, "rev-parse", "--show-toplevel")
	if e != nil {
		return snap, e
	}
	root := strings.TrimSpace(string(b))
	base := o["base"]
	if base == "" {
		base = "HEAD"
	}
	b, e = runGit(root, "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
	if e != nil {
		return snap, e
	}
	base = strings.TrimSpace(string(b))
	if !regexp.MustCompile(`^[a-f0-9]{40,64}$`).MatchString(base) {
		return snap, errors.New("invalid base revision")
	}
	if o["base"] != "" {
		b, e = runGit(root, "merge-base", base, "HEAD")
		if e != nil {
			return snap, e
		}
		base = strings.TrimSpace(string(b))
	}
	rules := []ignoreRule{}
	policy, e := readTree(root, base, ".inkspectignore")
	if e != nil {
		return snap, fmt.Errorf("base ignore policy: %w", e)
	}
	if policy != nil {
		rules, e = ignoreRules([]byte(*policy))
		if e != nil {
			return snap, e
		}
	}
	// Local exclusions can further restrict upload but never loosen the base policy.
	localPolicy, e := readWorking(root, ".inkspectignore")
	if e != nil {
		return snap, e
	}
	localRules := []ignoreRule{}
	if localPolicy != nil {
		localRules, e = ignoreRules([]byte(*localPolicy))
		if e != nil {
			return snap, e
		}
	}
	args := []string{"diff", "--no-ext-diff", "--no-textconv", "--name-only", "--no-renames", "-z"}
	if o["staged"] != "" {
		args = append(args, "--cached")
	}
	args = append(args, base, "--")
	b, e = runGit(root, args...)
	if e != nil {
		return snap, e
	}
	paths := strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00")
	if o["include-untracked"] != "" {
		b, e = runGit(root, "ls-files", "--others", "--exclude-standard", "-z")
		if e != nil {
			return snap, e
		}
		paths = append(paths, strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00")...)
	}
	sort.Strings(paths)
	seen := map[string]bool{}
	size := 0
	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		reason := ""
		if !safePath(p) {
			reason = "sensitive or unsupported path"
		} else if excluded(p, rules) || excluded(p, localRules) {
			reason = "excluded by .inkspectignore"
		}
		var before, after *string
		if reason == "" {
			before, e = readTree(root, base, p)
			if e != nil {
				reason = e.Error()
			}
		}
		if reason == "" {
			if o["staged"] != "" {
				after, e = readStaged(root, p)
			} else {
				after, e = readWorking(root, p)
			}
			if e != nil {
				reason = e.Error()
			}
		}
		if reason == "" {
			if before == nil && after == nil || before != nil && after != nil && *before == *after {
				continue
			}
			n := 0
			if before != nil {
				n += len(*before)
			}
			if after != nil {
				n += len(*after)
			}
			if len(snap.Files) >= 80 || size+n > 400000 {
				reason = "snapshot size limit"
			} else {
				size += n
				snap.Files = append(snap.Files, SnapshotFile{p, before, after, "changed"})
			}
		}
		if reason != "" {
			snap.Omissions = append(snap.Omissions, Omission{p, reason})
			if len(snap.Omissions) > 200 {
				return snap, errors.New("too many omitted files; narrow the review scope")
			}
		}
	}
	if len(snap.Files) == 0 {
		return snap, errors.New("no eligible changes to review")
	}
	snap.SnapshotID, e = digest(snap.Files)
	if e != nil {
		return snap, e
	}
	task := o["task"]
	if task == "" {
		task = "Review these changes for correctness, regressions and security issues. Cite evidence and report missing verification."
	}
	snap.Task = map[string]any{"id": snap.SnapshotID, "instructions": task, "criteria": []any{map[string]any{"id": "correctness", "text": "Changes preserve intended behavior and introduce no evidenced correctness or security regressions.", "verification": "source"}}}
	snap.Baseline = map[string]string{"kind": "revision", "reference": base}
	b, e = encodeCanonical(snap)
	if e != nil {
		return snap, e
	}
	if len(b) > 750000 {
		return snap, errors.New("encoded snapshot exceeds Cloud request limit")
	}
	return snap, nil
}
