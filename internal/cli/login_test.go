package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeviceLoginAndWhoami(t *testing.T) {
	t.Setenv("INKSPECT_API_KEY", "")
	t.Setenv("INKSPECT_API_URL", "")
	oldPath, oldWait := configPath, waitLoginPoll
	t.Cleanup(func() { configPath = oldPath; waitLoginPoll = oldWait })
	path := filepath.Join(t.TempDir(), "config", "credentials.json")
	configPath = func() (string, error) { return path, nil }
	var waits []time.Duration
	waitLoginPoll = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	token := "isk_live_" + strings.Repeat("a", 64)
	device := strings.Repeat("b", 64)
	code := "ABCD-1234-EF56"
	mode := "success"
	polls := 0
	var origin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/cli/device/start":
			if r.Header.Get("Authorization") != "" {
				t.Error("device start sent credentials")
			}
			link := origin + "/cli/authorize?code=" + code
			if mode == "unsafe" {
				link = "https://untrusted.invalid/cli/authorize?code=" + code
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"device_code": device, "user_code": code, "verification_uri_complete": link, "interval": 5, "expires_in": 600})
		case "/api/cli/device/poll":
			if r.Header.Get("Authorization") != "" {
				t.Error("poll sent credentials")
			}
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["device_code"] != device {
				t.Error("wrong device")
			}
			polls++
			status := "authorization_pending"
			if polls == 2 {
				status = "slow_down"
			}
			if mode == "denied" {
				status = "access_denied"
			}
			if polls >= 3 && mode == "success" {
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": token, "token_type": "Bearer"})
				return
			}
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": status})
		case "/api/cli/whoami":
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Error("missing scoped credential")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]string{"name": "Example", "email": "test@example.invalid"}, "workspace": map[string]string{"name": "Workspace", "slug": "workspace", "role": "owner"}})
		default:
			t.Error("unexpected route")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	origin = server.URL
	var out, log bytes.Buffer
	if e := Run([]string{"login", "--no-browser", "--api-url", origin}, strings.NewReader(""), &out, &log); e != nil {
		t.Fatal(e)
	}
	if len(waits) != 3 || waits[0] != 5*time.Second || waits[2] != 10*time.Second {
		t.Fatalf("poll intervals: %v", waits)
	}
	if strings.Contains(out.String()+log.String(), token) || strings.Contains(out.String()+log.String(), device) {
		t.Fatal("secret printed")
	}
	if !strings.Contains(log.String(), code) {
		t.Fatal("missing user code")
	}
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	out.Reset()
	log.Reset()
	if e := Run([]string{"whoami", "--format", "json"}, strings.NewReader(""), &out, &log); e != nil {
		t.Fatal(e)
	}
	if !json.Valid(out.Bytes()) || !strings.Contains(out.String(), "test@example.invalid") || log.Len() != 0 {
		t.Fatal("invalid whoami")
	}
	for _, next := range []string{"denied", "unsafe"} {
		mode = next
		polls = 0
		if e := Run([]string{"login", "--no-browser", "--api-url", origin}, strings.NewReader(""), &out, &log); e == nil {
			t.Fatal("unsafe/denied login succeeded")
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Fatal("failed login replaced credentials")
		}
		if next == "unsafe" && polls != 0 {
			t.Fatal("untrusted browser flow polled")
		}
	}
}
func TestLoginRequiresExplicitNoninteractiveMode(t *testing.T) {
	var out bytes.Buffer
	if e := Run([]string{"login"}, strings.NewReader(""), &out, &out); e == nil {
		t.Fatal("noninteractive login waited")
	}
	if e := Run([]string{"login", "--no-browser", "--token-stdin"}, strings.NewReader(""), &out, &out); e == nil {
		t.Fatal("conflicting modes accepted")
	}
}
