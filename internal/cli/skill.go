package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const skillOrigin = "https://inkspect.si"
const instructionBlock = "<!-- inkspect-cli:begin -->\nFor code discovery in this Inkspect-connected project, use the Inkspect skill in `.agents/skills/inkspect/SKILL.md`. Retrieve context before broad file searches, and verify local freshness before edits. Review commands require task authorization.\n<!-- inkspect-cli:end -->"

type skillManifest struct {
	Version string            `json:"version"`
	MinCLI  string            `json:"minimum_cli"`
	Files   map[string]string `json:"files"`
}

var download = func(raw string) ([]byte, error) {
	c := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := c.Get(raw)
	if e != nil {
		return nil, errors.New("skill download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("skill download returned HTTP %d; this release may not be published yet", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 128001))
	if e != nil || len(b) > 128000 {
		return nil, errors.New("skill asset exceeded limit")
	}
	return b, nil
}

func noSymlinkPath(path string) error {
	abs, e := filepath.Abs(path)
	if e != nil {
		return e
	}
	for p := abs; ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			return errors.New("skill and instruction paths must not contain symlinks")
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return nil
}
func skillCommand(pos []string, o options, out io.Writer) error {
	if len(pos) != 1 {
		return errors.New("choose skill install, update, list or uninstall")
	}
	if o["agent"] != "" && o["agent"] != "codex" {
		return errors.New("this release supports --agent codex")
	}
	if pos[0] == "updates" {
		return skillUpdates(o, out)
	}
	scope := o["scope"]
	if scope == "" {
		scope = "project"
	}
	if scope != "project" && scope != "global" {
		return errors.New("scope must be project or global")
	}
	_, root, e := project()
	if e != nil {
		return e
	}
	if scope == "global" {
		root, e = os.UserHomeDir()
		if e != nil {
			return e
		}
	}
	// Canonicalize the explicitly selected project/home root (macOS /var is a
	// system symlink). Reject links inside the skill/instruction paths below it.
	root, e = filepath.EvalSymlinks(root)
	if e != nil {
		return e
	}
	dir := filepath.Join(root, ".agents", "skills", "inkspect")
	if e = noSymlinkPath(dir); e != nil {
		return e
	}
	metaPath := filepath.Join(dir, ".inkspect-install.json")
	if e = noSymlinkPath(metaPath); e != nil {
		return e
	}
	var prior skillManifest
	b, readErr := os.ReadFile(metaPath)
	if readErr == nil {
		if json.Unmarshal(b, &prior) != nil {
			return errors.New("invalid skill install metadata")
		}
	}
	if pos[0] == "list" {
		return output(out, map[string]any{"installed": readErr == nil, "version": prior.Version, "path": dir, "scope": scope}, o["format"])
	}
	if pos[0] != "install" && pos[0] != "update" && pos[0] != "uninstall" {
		return errors.New("unknown skill operation")
	}
	if pos[0] == "update" || pos[0] == "uninstall" {
		if readErr != nil {
			return errors.New("no managed skill installed")
		}
	}
	if readErr == nil {
		for name, hash := range prior.Files {
			if name != "SKILL.md" {
				return errors.New("unexpected managed skill path")
			}
			p := filepath.Join(dir, name)
			if e = noSymlinkPath(p); e != nil {
				return e
			}
			data, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != hash {
				return errors.New("skill was edited locally; preserve or move your edits before updating/uninstalling")
			}
		}
	} else if !os.IsNotExist(readErr) {
		return readErr
	}
	agents := filepath.Join(root, "AGENTS.md")
	if scope == "project" {
		if e = noSymlinkPath(agents); e != nil {
			return e
		}
		if _, e = instructionContent(agents, pos[0] != "uninstall"); e != nil {
			return e
		}
	}
	if pos[0] == "uninstall" {
		if scope == "project" {
			if e = updateInstructions(agents, false); e != nil {
				return e
			}
		}
		for name := range prior.Files {
			if e = os.Remove(filepath.Join(dir, name)); e != nil {
				return e
			}
		}
		if e = os.Remove(metaPath); e != nil {
			return e
		}
		_ = os.Remove(dir)
		fmt.Fprintln(out, "Removed managed Inkspect skill; unrelated files preserved.")
		return nil
	}
	if readErr != nil {
		if _, e = os.Stat(dir); e == nil {
			return errors.New("skill directory already exists and is not managed by Inkspect")
		}
	}
	release, e := releaseMetadata(true)
	if e != nil {
		return e
	}
	if release == nil || !release.Skill.Published {
		return errors.New("no published skill release is available yet")
	}
	if newer(release.Skill.MinimumCLI, Version) {
		return errors.New("the latest skill requires a newer CLI; run inkspect update first")
	}
	base := skillOrigin + "/cli/skills/" + release.Skill.Version + "/"
	b, e = download(base + "manifest.json")
	if e != nil {
		return e
	}
	var manifest skillManifest
	if json.Unmarshal(b, &manifest) != nil || manifest.Version != release.Skill.Version || manifest.MinCLI != release.Skill.MinimumCLI || newer(manifest.MinCLI, Version) || len(manifest.Files) != 1 {
		return errors.New("incompatible skill manifest")
	}
	hash, ok := manifest.Files["SKILL.md"]
	if !ok || len(hash) != 64 {
		return errors.New("invalid skill file manifest")
	}
	data, e := download(base + "SKILL.md")
	if e != nil {
		return e
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != hash {
		return errors.New("skill checksum mismatch")
	}
	if e = os.MkdirAll(dir, 0755); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(dir, "SKILL.md"), data, 0644); e != nil {
		return e
	}
	b, _ = json.MarshalIndent(manifest, "", "  ")
	if e = os.WriteFile(metaPath, b, 0644); e != nil {
		return e
	}
	if scope == "project" {
		if e = updateInstructions(agents, true); e != nil {
			return e
		}
	}
	fmt.Fprintf(out, "Installed Inkspect skill %s in %s. Start a new agent session to discover it.\n", manifest.Version, dir)
	return nil
}
func updateInstructions(path string, install bool) error {
	s, e := instructionContent(path, install)
	if e != nil {
		return e
	}
	return os.WriteFile(path, []byte(s), 0644)
}
func instructionContent(path string, install bool) (string, error) {
	b, e := os.ReadFile(path)
	if e != nil && !os.IsNotExist(e) {
		return "", e
	}
	s := string(b)
	if strings.Contains(s, "<!-- inkspect-cli:begin -->") || strings.Contains(s, "<!-- inkspect-cli:end -->") {
		if !strings.Contains(s, instructionBlock) {
			return "", errors.New("Inkspect instruction block was edited; preserve it manually before changing the installation")
		}
		if !install {
			s = strings.Replace(s, instructionBlock, "", 1)
		}
	} else if install {
		if s != "" && !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		s += "\n" + instructionBlock + "\n"
	}
	return s, nil
}
