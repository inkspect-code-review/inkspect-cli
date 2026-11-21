package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSkillDownloadIntegrityAndUserEdits(t *testing.T) {
	t.Chdir(t.TempDir())
	info := testReleaseInfo(Version, Version, Version)
	mockReleaseMetadata(t, info)
	if e := os.WriteFile("AGENTS.md", []byte("Existing instructions.\n"), 0600); e != nil {
		t.Fatal(e)
	}
	old := download
	defer func() { download = old }()
	data := []byte("---\nname: inkspect\ndescription: Retrieve context.\n---\nUse the CLI.\n")
	manifest, _ := json.Marshal(skillManifest{Version, Version, map[string]string{"SKILL.md": fmt.Sprintf("%x", sha256.Sum256(data))}})
	download = func(url string) ([]byte, error) {
		if strings.HasSuffix(url, "manifest.json") {
			return manifest, nil
		}
		return []byte("tampered"), nil
	}
	var out bytes.Buffer
	if skillCommand([]string{"install"}, options{}, &out) == nil {
		t.Fatal("checksum mismatch accepted")
	}
	if _, e := os.Stat(".agents"); !os.IsNotExist(e) {
		t.Fatal("invalid download wrote files")
	}
	download = func(url string) ([]byte, error) {
		if strings.HasSuffix(url, "manifest.json") {
			return manifest, nil
		}
		return data, nil
	}
	if e := skillCommand([]string{"install"}, options{}, &out); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(".agents", "skills", "inkspect", "SKILL.md")
	if e := os.WriteFile(p, []byte("User edits"), 0644); e != nil {
		t.Fatal(e)
	}
	if skillCommand([]string{"update"}, options{}, &out) == nil {
		t.Fatal("user edits overwritten")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "User edits" {
		t.Fatal("user edits lost")
	}
}

func TestShellInstallerVerifiesArchiveBeforeWriting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer; Windows native binary runs in CI")
	}
	installer, e := filepath.Abs("../../scripts/install.sh")
	if e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	mock := filepath.Join(root, "mock")
	if e = os.Mkdir(mock, 0755); e != nil {
		t.Fatal(e)
	}
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	program := []byte("#!/bin/sh\nprintf '" + Version + "\\n'\n")
	if e = tw.WriteHeader(&tar.Header{Name: "inkspect", Mode: 0755, Size: int64(len(program))}); e != nil {
		t.Fatal(e)
	}
	_, _ = tw.Write(program)
	_ = tw.Close()
	_ = gz.Close()
	if e = os.WriteFile(filepath.Join(root, "archive"), archive.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(mock, "uname"), []byte("#!/bin/sh\ncase \"$1\" in -s) echo Linux;; *) echo x86_64;; esac\n"), 0755); e != nil {
		t.Fatal(e)
	}
	curl := `#!/bin/sh
case "$*" in *SHA256SUMS*) source="$FIXTURE_DIR/checksums";; *) source="$FIXTURE_DIR/archive";; esac
while [ "$#" -gt 0 ]; do if [ "$1" = '-o' ]; then shift; cp "$source" "$1"; exit; fi; shift; done
exit 1
`
	if e = os.WriteFile(filepath.Join(mock, "curl"), []byte(curl), 0755); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", mock+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FIXTURE_DIR", root)
	dest := filepath.Join(root, "bin")
	t.Setenv("INKSPECT_INSTALL_DIR", dest)
	if e = os.WriteFile(filepath.Join(root, "checksums"), []byte(strings.Repeat("0", 64)+"  inkspect_"+Version+"_linux_amd64.tar.gz\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = exec.Command("sh", installer).Run(); e == nil {
		t.Fatal("bad archive checksum installed")
	}
	if _, e = os.Stat(dest); !os.IsNotExist(e) {
		t.Fatal("destination changed before verification")
	}
	valid := fmt.Sprintf("%x  inkspect_%s_linux_amd64.tar.gz\n", sha256.Sum256(archive.Bytes()), Version)
	if e = os.WriteFile(filepath.Join(root, "checksums"), []byte(valid), 0600); e != nil {
		t.Fatal(e)
	}
	if b, e := exec.Command("sh", installer).CombinedOutput(); e != nil {
		t.Fatalf("installer: %v: %s", e, b)
	}
	b, e := exec.Command(filepath.Join(dest, "inkspect"), "--version").Output()
	if e != nil || strings.TrimSpace(string(b)) != Version {
		t.Fatal("installed executable failed")
	}
}

func TestHTTPDoesNotRedirectCredentialsOrRetry(t *testing.T) {
	hits := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("Authorization") != "Bearer test" {
			t.Error("missing auth")
		}
		w.Header().Set("Location", "http://example.invalid")
		w.WriteHeader(307)
		fmt.Fprint(w, `{"error":"moved"}`)
	}))
	defer s.Close()
	c := Client{Config{s.URL, "test"}, httpClient()}
	if _, e := c.request("POST", "/api/cli/context", map[string]string{"question": "test"}); e == nil {
		t.Fatal("redirect accepted")
	}
	if hits != 1 {
		t.Fatal("request was retried")
	}
}
func TestOrigins(t *testing.T) {
	for _, s := range []string{"http://evil.test", "https://user:pass@example.com", "https://example.com/path", "https://example.com?token=x"} {
		if validateOrigin(s) == nil {
			t.Errorf("accepted %s", s)
		}
	}
	for _, s := range []string{"https://app.inkspect.si", "http://127.0.0.1:1234"} {
		if validateOrigin(s) != nil {
			t.Errorf("rejected %s", s)
		}
	}
}
func TestCredentialStorageAndReplacement(t *testing.T) {
	p := filepath.Join(t.TempDir(), "inkspect", "credentials.json")
	old := configPath
	defer func() { configPath = old }()
	configPath = func() (string, error) { return p, nil }
	for _, token := range []string{"first-test-value", "replacement-test-value"} {
		if e := saveConfig(Config{DefaultOrigin, token}); e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		var c Config
		if json.Unmarshal(b, &c) != nil || c.Token != token {
			t.Fatal("credential save did not round-trip")
		}
	}
	if runtime.GOOS != "windows" {
		info, e := os.Stat(p)
		if e != nil {
			t.Fatal(e)
		}
		if info.Mode().Perm()&0077 != 0 {
			t.Fatal("credentials readable by other users")
		}
	}
}
func TestNoninteractiveInput(t *testing.T) {
	t.Setenv("INKSPECT_API_KEY", "test")
	var out bytes.Buffer
	if Run([]string{"ask"}, strings.NewReader(""), &out, &out) == nil {
		t.Fatal("noninteractive ask waited")
	}
	if _, _, e := parse([]string{"ask", "--budegt", "3"}); e == nil {
		t.Fatal("unknown flag accepted")
	}
	if _, e := readInput(strings.NewReader("12345"), 4); e == nil {
		t.Fatal("unbounded input")
	}
	if Run([]string{"review", "pr", "123", "--dry-run"}, strings.NewReader(""), &out, &out) == nil {
		t.Fatal("ignored safety flag on paid operation")
	}
}
func TestSafeSources(t *testing.T) {
	for _, p := range []string{".env.local", "a/../b", "src/link\\file", "keys/key.pem", "node_modules/x.js", ".git/config", "secrets/token.txt", "C:/file"} {
		if safePath(p) {
			t.Error(p)
		}
	}
	if !safePath("src/payment.ts") {
		t.Fatal("source rejected")
	}
	if _, e := textSource([]byte{0, 1}); e == nil {
		t.Fatal("binary accepted")
	}
	if _, e := textSource([]byte("-----BEGIN PRIVATE KEY-----")); e == nil {
		t.Fatal("key accepted")
	}
}
func TestIgnorePolicy(t *testing.T) {
	r, e := ignoreRules([]byte("dist/\n!dist/keep.ts\n/docs/**/*.png\n*.min.js\n"))
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{"a/dist/a.js", "docs/a/b.png", "docs/a.png", "app.min.js"} {
		if !excluded(p, r) {
			t.Error(p)
		}
	}
	for _, p := range []string{"dist/keep.ts", "src/main.ts", ".inkspectignore"} {
		if excluded(p, r) {
			t.Error(p)
		}
	}
	for _, p := range []string{"[abc]", "src/**x/a", "a/../b", "foo\\bar"} {
		if _, e = ignoreRules([]byte(p)); e == nil {
			t.Errorf("invalid ignore accepted %s", p)
		}
	}
}
func TestDigestMatchesJSONContract(t *testing.T) {
	a := "<>&\u2028"
	files := []SnapshotFile{{"z.ts", nil, &a, "changed"}}
	got, e := digest(files)
	if e != nil {
		t.Fatal(e)
	}
	want := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("[{\"path\":\"z.ts\",\"before\":null,\"after\":\"<>&\u2028\",\"role\":\"changed\"}]")))
	if got != want {
		t.Fatalf("digest %s != %s", got, want)
	}
}
func TestWorkingSymlinkRejected(t *testing.T) {
	root := t.TempDir()
	if e := os.Symlink(t.TempDir(), filepath.Join(root, "linked")); e != nil {
		t.Skip("symlink creation unavailable")
	}
	if _, e := readWorking(root, "linked/code.ts"); e == nil {
		t.Fatal("symlink traversed")
	}
}
func TestMCPFramingAndLifecycle(t *testing.T) {
	var out, log bytes.Buffer
	c := &Client{Config{"https://unused.invalid", "test"}, httpClient()}
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}
{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":3,"method":"tools/list"}
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"inkspect_context","arguments":{"question":4}}}
not-json
`
	if e := serveMCP(c, strings.NewReader(input), &out, &log); e != nil {
		t.Fatal(e)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("bad framing %s", out.String())
	}
	for _, line := range lines {
		var v map[string]any
		if json.Unmarshal([]byte(line), &v) != nil || v["jsonrpc"] != "2.0" {
			t.Fatal("stdout polluted")
		}
	}
	if !strings.Contains(lines[0], "Initialize first") || !strings.Contains(lines[2], "inkspect_source") || !strings.Contains(lines[3], `"isError":true`) {
		t.Fatal(out.String())
	}
}
func TestInstructionPreservation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "AGENTS.md")
	original := "# My rules\nDo not commit.\n"
	if e := os.WriteFile(p, []byte(original), 0600); e != nil {
		t.Fatal(e)
	}
	if e := updateInstructions(p, true); e != nil {
		t.Fatal(e)
	}
	if e := updateInstructions(p, true); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(p)
	if strings.Count(string(b), "inkspect-cli:begin") != 1 {
		t.Fatal("duplicate block")
	}
	if e := updateInstructions(p, false); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(p)
	if !strings.HasPrefix(string(b), original) {
		t.Fatal("user rules changed")
	}
}
func TestSnapshotUsesStagedBlobAndOmitsSecrets(t *testing.T) {
	root := t.TempDir()
	old := runGit
	defer func() { runGit = old }()
	head := strings.Repeat("a", 40)
	runGit = func(_ string, args ...string) ([]byte, error) {
		s := strings.Join(args, " ")
		switch {
		case s == "rev-parse --show-toplevel":
			return []byte(root), nil
		case strings.HasPrefix(s, "rev-parse --verify"):
			return []byte(head), nil
		case strings.HasPrefix(s, "diff "):
			if !strings.Contains(s, "--cached") {
				t.Fatal("not staged")
			}
			return []byte("src/a.ts\x00.env\x00"), nil
		case strings.HasPrefix(s, "ls-tree"):
			if strings.HasSuffix(s, ".inkspectignore") {
				return nil, nil
			}
			return []byte("100644 blob old\tsrc/a.ts\x00"), nil
		case strings.HasPrefix(s, "ls-files --stage"):
			return []byte("100644 new 0\tsrc/a.ts\x00"), nil
		case s == "cat-file blob old":
			return []byte("before"), nil
		case s == "cat-file blob new":
			return []byte("staged"), nil
		default:
			t.Fatalf("unexpected Git read %s", s)
			return nil, nil
		}
	}
	snap, e := collectSnapshot(options{"staged": "true"})
	if e != nil {
		t.Fatal(e)
	}
	if len(snap.Files) != 1 || *snap.Files[0].After != "staged" || len(snap.Omissions) != 1 {
		t.Fatalf("bad snapshot %#v", snap)
	}
}
func TestContextFreshnessRequiresMatchingRepository(t *testing.T) {
	old := runGit
	defer func() { runGit = old }()
	head := strings.Repeat("a", 40)
	runGit = func(_ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "rev-parse":
			return []byte(head), nil
		case "status":
			return nil, nil
		default:
			return []byte("git@github.com:other/repo.git"), nil
		}
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"sources":[{"revision":%q}],"incomplete":false}`, head)
	}))
	defer s.Close()
	c := &Client{Config{s.URL, "test"}, httpClient()}
	v, e := ask(c, "payments", options{"repo": "owner/repo"})
	if e != nil {
		t.Fatal(e)
	}
	if v["freshness"] != "unknown" {
		t.Fatal("unrelated checkout marked current")
	}
	if _, e = ask(c, "payments", options{"repo": "owner/repo", "require-current": "true"}); e == nil {
		t.Fatal("unknown freshness accepted")
	}
}
