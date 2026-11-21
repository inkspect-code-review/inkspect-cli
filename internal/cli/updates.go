package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type releaseInfo struct {
	SchemaVersion int `json:"schema_version"`
	CLI           struct {
		Version   string `json:"version"`
		Published bool   `json:"published"`
	} `json:"cli"`
	Skill struct {
		Version    string `json:"version"`
		MinimumCLI string `json:"minimum_cli"`
		Published  bool   `json:"published"`
	} `json:"skill"`
}
type releaseCache struct {
	CheckedAt int64        `json:"checked_at"`
	Info      *releaseInfo `json:"info"`
}

var updateCachePath = func() (string, error) {
	root, e := os.UserCacheDir()
	return filepath.Join(root, "inkspect", "releases.json"), e
}
var fetchReleaseInfo = func() (*releaseInfo, error) {
	b, e := publicDownload(skillOrigin+"/cli/releases.json", 16384, 2*time.Second, false)
	if e != nil {
		return nil, e
	}
	var v releaseInfo
	if json.Unmarshal(b, &v) != nil || v.SchemaVersion != 1 || !validVersion(v.CLI.Version) || !validVersion(v.Skill.Version) || !validVersion(v.Skill.MinimumCLI) {
		return nil, errors.New("invalid release information")
	}
	return &v, nil
}

func validVersion(v string) bool {
	return regexp.MustCompile(`^(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})$`).MatchString(v)
}
func newer(a, b string) bool {
	if !validVersion(a) || !validVersion(b) {
		return false
	}
	aa := strings.Split(a, ".")
	bb := strings.Split(b, ".")
	for i := range aa {
		x, _ := strconv.Atoi(aa[i])
		y, _ := strconv.Atoi(bb[i])
		if x != y {
			return x > y
		}
	}
	return false
}
func releaseMetadata(force bool) (*releaseInfo, error) {
	p, e := updateCachePath()
	if e != nil {
		return nil, e
	}
	if !force {
		if b, e := os.ReadFile(p); e == nil && len(b) < 32768 {
			var cache releaseCache
			if json.Unmarshal(b, &cache) == nil && cache.CheckedAt <= time.Now().Unix() && time.Now().Unix()-cache.CheckedAt < 86400 {
				return cache.Info, nil
			}
		}
	}
	info, err := fetchReleaseInfo()
	if os.MkdirAll(filepath.Dir(p), 0700) == nil {
		b, _ := json.Marshal(releaseCache{time.Now().Unix(), info})
		_ = os.WriteFile(p, b, 0600)
	}
	return info, err
}
func installedSkill(scope string) (skillManifest, error) {
	_, root, e := project()
	if e != nil {
		return skillManifest{}, e
	}
	if scope == "global" {
		root, e = os.UserHomeDir()
		if e != nil {
			return skillManifest{}, e
		}
	}
	b, e := os.ReadFile(filepath.Join(root, ".agents", "skills", "inkspect", ".inkspect-install.json"))
	if e != nil {
		return skillManifest{}, e
	}
	var m skillManifest
	if len(b) > 32768 || json.Unmarshal(b, &m) != nil {
		return m, errors.New("invalid installed skill metadata")
	}
	return m, nil
}
func notifyUpdates(log io.Writer) {
	if os.Getenv("INKSPECT_NO_UPDATE_CHECK") != "" {
		return
	}
	info, e := releaseMetadata(false)
	if e != nil || info == nil {
		return
	}
	if info.CLI.Published && newer(info.CLI.Version, Version) {
		fmt.Fprintf(log, "Inkspect %s is available (installed: %s). Update with: inkspect update\n", info.CLI.Version, Version)
	}
	if info.Skill.Published {
		for _, scope := range []string{"project", "global"} {
			installed, e := installedSkill(scope)
			if e != nil || !newer(info.Skill.Version, installed.Version) {
				continue
			}
			if newer(info.Skill.MinimumCLI, Version) {
				fmt.Fprintf(log, "Inkspect skill %s requires a newer CLI. Run: inkspect update\n", info.Skill.Version)
			} else {
				suffix := ""
				if scope == "global" {
					suffix = " --scope global"
				}
				fmt.Fprintf(log, "Inkspect skill %s is available (%s). Check with: inkspect skill updates%s; install with: inkspect skill update%s\n", info.Skill.Version, scope, suffix, suffix)
			}
		}
	}
}
func skillUpdates(o options, out io.Writer) error {
	info, e := releaseMetadata(true)
	if e != nil {
		return e
	}
	if info == nil {
		return errors.New("release information unavailable")
	}
	scope := o["scope"]
	if scope == "" {
		scope = "project"
	}
	if scope != "project" && scope != "global" {
		return errors.New("scope must be project or global")
	}
	installed, err := installedSkill(scope)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	command := "inkspect skill update"
	if err != nil {
		command = "inkspect skill install"
	}
	if scope == "global" {
		command += " --scope global"
	}
	compatible := !newer(info.Skill.MinimumCLI, Version)
	if !compatible {
		command = "inkspect update"
	}
	return output(out, map[string]any{"installed_version": installed.Version, "latest_version": info.Skill.Version, "published": info.Skill.Published, "update_available": err == nil && info.Skill.Published && newer(info.Skill.Version, installed.Version), "compatible": compatible, "command": command}, o["format"])
}
func publicDownload(raw string, limit int64, timeout time.Duration, redirects bool) ([]byte, error) {
	c := &http.Client{Timeout: timeout, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if !redirects {
			return http.ErrUseLastResponse
		}
		if r.URL.Scheme != "https" || r.URL.User != nil || len(via) >= 8 {
			return errors.New("unsafe download redirect")
		}
		return nil
	}}
	response, e := c.Get(raw)
	if e != nil {
		return nil, errors.New("could not reach the release server")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("release download returned HTTP %d", response.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if e != nil || int64(len(b)) > limit {
		return nil, errors.New("release download exceeded size limit")
	}
	return b, nil
}
func releaseBinary(archive []byte, name string) ([]byte, error) {
	var r io.Reader
	if strings.HasSuffix(name, ".exe") {
		z, e := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if e != nil {
			return nil, e
		}
		if len(z.File) != 1 || z.File[0].Name != name || z.File[0].Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("unexpected release archive entries")
		}
		f, e := z.File[0].Open()
		if e != nil {
			return nil, e
		}
		defer f.Close()
		r = f
	} else {
		gz, e := gzip.NewReader(bytes.NewReader(archive))
		if e != nil {
			return nil, e
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		h, e := tr.Next()
		if e != nil {
			return nil, e
		}
		if h.Name != name || h.Typeflag != tar.TypeReg {
			return nil, errors.New("unexpected release archive entry")
		}
		b, e := io.ReadAll(io.LimitReader(tr, 40_000_001))
		if e != nil || len(b) > 40_000_000 {
			return nil, errors.New("release binary exceeds size limit")
		}
		if _, e = tr.Next(); e != io.EOF {
			return nil, errors.New("unexpected release archive entries")
		}
		return b, nil
	}
	b, e := io.ReadAll(io.LimitReader(r, 40_000_001))
	if e != nil || len(b) > 40_000_000 {
		return nil, errors.New("release binary exceeds size limit")
	}
	return b, nil
}
func replaceExecutable(target string, data []byte) error {
	if info, e := os.Lstat(target); e != nil || !info.Mode().IsRegular() {
		return errors.New("current executable must be a regular file")
	}
	f, e := os.CreateTemp(filepath.Dir(target), ".inkspect-update-*")
	if e != nil {
		return errors.New("executable directory is not writable; use the installer in a user directory")
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Chmod(0755); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if runtime.GOOS != "windows" {
		return os.Rename(tmp, target)
	}
	// Windows cannot overwrite a running executable. Move it aside first; a
	// unique backup is preserved if Windows still holds it open. Never overwrite
	// an existing user file or delete unverified paths during a future startup.
	backup := target + ".previous-" + filepath.Base(tmp)
	if e = os.Rename(target, backup); e != nil {
		return errors.New("Windows could not move the running executable; close other Inkspect processes and retry")
	}
	if e = os.Rename(tmp, target); e != nil {
		_ = os.Rename(backup, target)
		return e
	}
	_ = os.Remove(backup)
	return nil
}
func updateCLI(out io.Writer) error {
	info, e := releaseMetadata(true)
	if e != nil {
		return e
	}
	if info == nil || !info.CLI.Published {
		return errors.New("no published CLI release is available yet")
	}
	if !newer(info.CLI.Version, Version) {
		fmt.Fprintln(out, "Inkspect is up to date ("+Version+").")
		return nil
	}
	if (runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows") || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return errors.New("unsupported release target")
	}
	ext := ".tar.gz"
	name := "inkspect"
	if runtime.GOOS == "windows" {
		ext = ".zip"
		name += ".exe"
	}
	asset := fmt.Sprintf("inkspect_%s_%s_%s%s", info.CLI.Version, runtime.GOOS, runtime.GOARCH, ext)
	base := "https://inkspect.si/cli/releases/v" + url.PathEscape(info.CLI.Version) + "/"
	sums, e := publicDownload(base+"SHA256SUMS", 16384, 30*time.Second, true)
	if e != nil {
		return e
	}
	expected := ""
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == asset {
			if expected != "" {
				return errors.New("duplicate release checksum")
			}
			expected = fields[0]
		}
	}
	if len(expected) != 64 {
		return errors.New("release checksum unavailable")
	}
	archive, e := publicDownload(base+asset, 64_000_000, 90*time.Second, true)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != expected {
		return errors.New("release checksum mismatch; installed executable was not changed")
	}
	binary, e := releaseBinary(archive, name)
	if e != nil {
		return e
	}
	if len(binary) < 4 {
		return errors.New("empty release executable")
	}
	target, e := os.Executable()
	if e != nil {
		return e
	}
	target, e = filepath.EvalSymlinks(target)
	if e != nil {
		return e
	}
	if e = replaceExecutable(target, binary); e != nil {
		return e
	}
	fmt.Fprintf(out, "Updated Inkspect to %s. Restart running MCP sessions to use it.\n", info.CLI.Version)
	return nil
}
