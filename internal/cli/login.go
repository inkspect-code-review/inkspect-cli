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
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var openLoginBrowser = func(link string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", link)
	case "windows":
		cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", link)
	default:
		cmd = exec.Command("xdg-open", link)
	}
	if e := cmd.Start(); e != nil {
		return e
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
var waitLoginPoll = func(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func loginOrigin(o options) (string, error) {
	origin := o["api-url"]
	if origin == "" {
		origin = os.Getenv("INKSPECT_API_URL")
	}
	if origin == "" {
		origin = DefaultOrigin
	}
	return origin, validateOrigin(origin)
}
func deviceRequest(ctx context.Context, client *http.Client, origin, path string, body any) (map[string]any, error) {
	data, e := json.Marshal(body)
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(origin, "/")+"/api/cli/device/"+path, bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "inkspect-cli/"+Version)
	resp, e := client.Do(req)
	if e != nil {
		return nil, errors.New("sign-in request failed; run inkspect login again when connected")
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, 16385))
	if e != nil || len(b) > 16384 {
		return nil, errors.New("invalid sign-in response")
	}
	var value map[string]any
	if json.Unmarshal(b, &value) != nil {
		return nil, errors.New("invalid sign-in response")
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return value, nil
	}
	code, _ := value["error"].(string)
	if path == "poll" && resp.StatusCode == 400 && (code == "authorization_pending" || code == "slow_down" || code == "access_denied" || code == "expired_token") {
		return value, nil
	}
	return nil, fmt.Errorf("sign-in failed (HTTP %d); check Cloud access and restart inkspect login", resp.StatusCode)
}
func browserLogin(o options, in io.Reader, out, log io.Writer) error {
	if !isTerminal(in) && o["no-browser"] == "" {
		return errors.New("noninteractive login requires --no-browser for device approval, --token-stdin or INKSPECT_API_KEY")
	}
	if os.Getenv("INKSPECT_API_KEY") != "" {
		return errors.New("INKSPECT_API_KEY overrides saved login; unset it before signing in through the browser")
	}
	origin, e := loginOrigin(o)
	if e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 10*time.Minute)
	defer timeout()
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	value, e := deviceRequest(ctx, client, origin, "start", map[string]any{})
	if e != nil {
		return e
	}
	device, _ := value["device_code"].(string)
	code, _ := value["user_code"].(string)
	link, _ := value["verification_uri_complete"].(string)
	interval, ok := value["interval"].(float64)
	expires, expiryOK := value["expires_in"].(float64)
	expected, _ := url.Parse(origin)
	verification, parseErr := url.Parse(link)
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(device) || !regexp.MustCompile(`^[A-F0-9]{4}-[A-F0-9]{4}-[A-F0-9]{4}$`).MatchString(code) || !ok || interval < 5 || interval > 60 || !expiryOK || expires < 1 || expires > 600 || parseErr != nil || verification.Scheme != expected.Scheme || verification.Host != expected.Host || verification.User != nil || verification.Path != "/cli/authorize" || verification.Fragment != "" || verification.RawQuery != (url.Values{"code": {code}}).Encode() {
		return errors.New("invalid or untrusted sign-in response")
	}
	deadline, cancelDeadline := context.WithTimeout(ctx, time.Duration(expires)*time.Second)
	defer cancelDeadline()
	fmt.Fprintf(log, "Open %s\nTerminal code: %s\nApprove only if this code matches. Waiting for approval…\n", link, code)
	if o["no-browser"] == "" {
		if e = openLoginBrowser(link); e != nil {
			fmt.Fprintln(log, "Could not open the browser. Open the link above on any device.")
		}
	}
	for {
		if e = waitLoginPoll(deadline, time.Duration(interval)*time.Second); e != nil {
			return errors.New("sign-in cancelled or expired; run inkspect login to start again")
		}
		reply, err := deviceRequest(deadline, client, origin, "poll", map[string]any{"device_code": device})
		if err != nil {
			return err
		}
		switch reply["error"] {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5
			continue
		case "access_denied":
			return errors.New("CLI access was declined or revoked; no credentials were changed")
		case "expired_token":
			return errors.New("sign-in expired or was already completed; run inkspect login again")
		}
		token, _ := reply["access_token"].(string)
		if reply["token_type"] != "Bearer" || !regexp.MustCompile(`^isk_live_[a-f0-9]{64}$`).MatchString(token) {
			return errors.New("invalid sign-in credentials received")
		}
		c := &Client{Config{origin, token}, client}
		if _, e = c.request("GET", "/api/cli/whoami", nil); e != nil {
			return e
		}
		if e = saveConfig(c.Config); e != nil {
			return e
		}
		fmt.Fprintln(out, "Signed in. Run inkspect whoami to see your account and workspace.")
		return nil
	}
}
func whoami(c *Client, o options, out io.Writer) error {
	value, e := c.request("GET", "/api/cli/whoami", nil)
	if e != nil {
		return e
	}
	if o["format"] == "json" {
		return output(out, value, "json")
	}
	user, _ := value["user"].(map[string]any)
	workspace, _ := value["workspace"].(map[string]any)
	credential, _ := value["credential"].(map[string]any)
	if user == nil || workspace == nil {
		return errors.New("invalid account response")
	}
	fmt.Fprintf(out, "Account: %s <%s>\nWorkspace: %s (%s)\nRole: %s\n", terminalText(fmt.Sprint(user["name"])), terminalText(fmt.Sprint(user["email"])), terminalText(fmt.Sprint(workspace["name"])), terminalText(fmt.Sprint(workspace["slug"])), terminalText(fmt.Sprint(workspace["role"])))
	if credential != nil {
		fmt.Fprintf(out, "Access expires: %s\n", terminalText(fmt.Sprint(credential["expires_at"])))
	}
	return nil
}
