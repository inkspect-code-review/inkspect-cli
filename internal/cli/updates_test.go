package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testReleaseInfo(cli, skill, minimum string) *releaseInfo {
	v := &releaseInfo{SchemaVersion: 1}
	v.CLI.Version = cli
	v.CLI.Published = true
	v.Skill.Version = skill
	v.Skill.MinimumCLI = minimum
	v.Skill.Published = true
	return v
}
func mockReleaseMetadata(t *testing.T, v *releaseInfo) {
	t.Helper()
	oldFetch, oldPath := fetchReleaseInfo, updateCachePath
	t.Cleanup(func() { fetchReleaseInfo = oldFetch; updateCachePath = oldPath })
	p := filepath.Join(t.TempDir(), "releases.json")
	updateCachePath = func() (string, error) { return p, nil }
	fetchReleaseInfo = func() (*releaseInfo, error) { return v, nil }
}
func TestUpdateVersionsAndCache(t *testing.T) {
	if !newer("1.10.0", "1.9.9") || newer("1.0.0", "1.0.0") || newer("1.0.0", "2.0.0") || validVersion("../1.0.0") {
		t.Fatal("invalid version comparison")
	}
	info := testReleaseInfo("0.2.0", "0.2.0", Version)
	mockReleaseMetadata(t, info)
	calls := 0
	fetchReleaseInfo = func() (*releaseInfo, error) { calls++; return info, nil }
	for range 2 {
		if _, e := releaseMetadata(false); e != nil {
			t.Fatal(e)
		}
	}
	if calls != 1 {
		t.Fatal("update check not cached")
	}
	if _, e := releaseMetadata(true); e != nil {
		t.Fatal(e)
	}
	if calls != 2 {
		t.Fatal("explicit check did not refresh")
	}
}
func TestUpdateNoticesAndMachineOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	info := testReleaseInfo("0.2.0", "0.2.0", Version)
	mockReleaseMetadata(t, info)
	dir := filepath.Join(".agents", "skills", "inkspect")
	if e := os.MkdirAll(dir, 0755); e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(skillManifest{Version, Version, nil})
	if e := os.WriteFile(filepath.Join(dir, ".inkspect-install.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	var log bytes.Buffer
	notifyUpdates(&log)
	if !strings.Contains(log.String(), "inkspect update") || !strings.Contains(log.String(), "inkspect skill updates") {
		t.Fatal("missing update instructions")
	}
	fetchReleaseInfo = func() (*releaseInfo, error) { t.Fatal("machine command checked updates"); return nil, nil }
	var out bytes.Buffer
	log.Reset()
	if e := Run([]string{"skill", "list", "--format", "json"}, strings.NewReader(""), &out, &log); e != nil {
		t.Fatal(e)
	}
	if !json.Valid(out.Bytes()) || log.Len() != 0 {
		t.Fatal("machine output polluted")
	}
}
func TestSkillUpdateCompatibility(t *testing.T) {
	t.Chdir(t.TempDir())
	mockReleaseMetadata(t, testReleaseInfo("1.0.0", "2.0.0", "1.0.0"))
	var out bytes.Buffer
	if e := skillCommand([]string{"install"}, options{}, &out); e == nil || !strings.Contains(e.Error(), "inkspect update") {
		t.Fatal("incompatible skill installed")
	}
	if e := skillUpdates(options{"format": "json"}, &out); e != nil {
		t.Fatal(e)
	}
	var status map[string]any
	if json.Unmarshal(out.Bytes(), &status) != nil || status["compatible"] != false || status["command"] != "inkspect update" {
		t.Fatal("missing compatibility guidance")
	}
}
func TestUnpublishedReleaseCannotUpdate(t *testing.T) {
	info := testReleaseInfo("1.0.0", Version, Version)
	info.CLI.Published = false
	info.Skill.Published = false
	mockReleaseMetadata(t, info)
	var out bytes.Buffer
	if updateCLI(&out) == nil {
		t.Fatal("unpublished release installed")
	}
	notifyUpdates(&out)
	if out.Len() != 0 {
		t.Fatal("unpublished release advertised")
	}
}
func TestLiveExecutableReplacement(t *testing.T) {
	current, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(current)
	if e != nil {
		t.Fatal(e)
	}
	if os.Getenv("INKSPECT_REPLACE_TEST_HELPER") == "1" {
		if e = replaceExecutable(current, data); e != nil {
			t.Fatal(e)
		}
		return
	}
	target := filepath.Join(t.TempDir(), filepath.Base(current))
	if e = os.WriteFile(target, data, 0755); e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(target, "-test.run=^TestLiveExecutableReplacement$")
	cmd.Env = append(os.Environ(), "INKSPECT_REPLACE_TEST_HELPER=1")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("running executable replacement: %v: %s", e, b)
	}
	replaced, e := os.ReadFile(target)
	if e != nil || sha256.Sum256(replaced) != sha256.Sum256(data) {
		t.Fatal("replacement executable changed unexpectedly")
	}
}
