package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const fixtureIndexID = "11111111-1111-4111-8111-111111111111"

func indexFixture(state string) map[string]any {
	return map[string]any{"repository": "owner/repo", "index": map[string]any{"id": fixtureIndexID, "status": state, "progress": map[string]any{"stage": state, "indexed": state == "completed", "chunks": float64(20), "embedded": float64(5), "reused": float64(15)}}}
}

func TestIndexTracksReturnedOperationAndKeepsJSONClean(t *testing.T) {
	t.Chdir(t.TempDir())
	old := waitIndexPoll
	t.Cleanup(func() { waitIndexPoll = old })
	waitIndexPoll = func(context.Context) error { return nil }
	posts, gets := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if !indexIDPattern.MatchString(fmt.Sprint(body["request_id"])) || body["repository"] != "owner/repo" {
				t.Error("invalid admission")
			}
			json.NewEncoder(w).Encode(indexFixture("queued"))
			return
		}
		gets++
		if r.URL.Query().Get("id") != fixtureIndexID {
			t.Error("polled a new/latest operation instead of the accepted ID")
		}
		state := "running"
		if gets == 2 {
			state = "completed"
		}
		json.NewEncoder(w).Encode(indexFixture(state))
	}))
	defer server.Close()
	var out, log bytes.Buffer
	err := indexCommand(&Client{Config{server.URL, "test"}, server.Client()}, nil, options{"repo": "owner/repo", "format": "json"}, strings.NewReader(""), &out, &log)
	if err != nil {
		t.Fatal(err)
	}
	if posts != 1 || gets != 2 || !json.Valid(out.Bytes()) || log.Len() != 0 {
		t.Fatalf("posts=%d polls=%d stdout=%s stderr=%s", posts, gets, out.String(), log.String())
	}
}

func TestIndexNoWaitStatusAndFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	state := "queued"
	gets := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			gets++
			if r.URL.Query().Get("id") != "" {
				t.Error("status should request latest operation")
			}
		}
		json.NewEncoder(w).Encode(indexFixture(state))
	}))
	defer server.Close()
	client := &Client{Config{server.URL, "test"}, server.Client()}
	var out, log bytes.Buffer
	if err := indexCommand(client, nil, options{"repo": "owner/repo", "no-wait": "true"}, strings.NewReader(""), &out, &log); err != nil {
		t.Fatal(err)
	}
	if gets != 0 {
		t.Fatal("no-wait polled")
	}
	if err := indexCommand(client, []string{"status"}, options{"repo": "owner/repo"}, strings.NewReader(""), &out, &log); err != nil {
		t.Fatal(err)
	}
	state = "failed"
	if err := indexCommand(client, nil, options{"repo": "owner/repo"}, strings.NewReader(""), &out, &log); err == nil {
		t.Fatal("failed index reported success")
	}
}

func TestIndexCancellationNeverCancelsCloudOrRetriesAdmission(t *testing.T) {
	t.Chdir(t.TempDir())
	old := waitIndexPoll
	t.Cleanup(func() { waitIndexPoll = old })
	waitIndexPoll = func(context.Context) error { return context.Canceled }
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/cli/index/start" {
			t.Error("unexpected mutation after cancellation")
		}
		json.NewEncoder(w).Encode(indexFixture("running"))
	}))
	defer server.Close()
	var out, log bytes.Buffer
	err := indexCommand(&Client{Config{server.URL, "test"}, server.Client()}, nil, options{"repo": "owner/repo", "format": "json"}, strings.NewReader(""), &out, &log)
	if err == nil || !strings.Contains(err.Error(), "Cloud work was not cancelled") || calls != 1 || !json.Valid(out.Bytes()) {
		t.Fatalf("%v calls=%d output=%s", err, calls, out.String())
	}
}

func TestIndexRequiresSingleRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	old := runGit
	t.Cleanup(func() { runGit = old })
	runGit = func(string, ...string) ([]byte, error) { return nil, errors.New("no checkout") }
	var out bytes.Buffer
	err := indexCommand(&Client{}, nil, options{}, strings.NewReader(""), &out, &out)
	if err == nil || !strings.Contains(err.Error(), "choose one repository") {
		t.Fatal(err)
	}
}

func TestSpinnerClearsLineAndNeverPollutesMachineStreams(t *testing.T) {
	var out bytes.Buffer
	stop := startSpinner(&out, "Thinking...", true)
	time.Sleep(150 * time.Millisecond)
	stop()
	stop()
	text := out.String()
	if !strings.Contains(text, "| Thinking...") || !strings.Contains(text, "/ Thinking...") || !strings.HasSuffix(text, "\r"+strings.Repeat(" ", 13)+"\r") {
		t.Fatalf("uncleared progress: %q", text)
	}
	if terminalProgress(options{}, strings.NewReader(""), &out, &out) || terminalProgress(options{"format": "json"}, strings.NewReader(""), &out, &out) {
		t.Fatal("machine output enabled progress")
	}
	out.Reset()
	startSpinner(&out, "Thinking...", false)()
	if out.Len() != 0 {
		t.Fatal("disabled spinner wrote output")
	}
}

func TestAskCancellationStopsHTTPAndReturnsNoPartialContext(t *testing.T) {
	t.Chdir(t.TempDir())
	started := make(chan struct{})
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(done)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-started; cancel() }()
	value, err := askContext(ctx, &Client{Config{server.URL, "test"}, server.Client()}, "question", options{"repo": "owner/repo"})
	if !errors.Is(err, context.Canceled) || value != nil {
		t.Fatalf("value=%v err=%v", value, err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("HTTP request did not cancel")
	}
}
