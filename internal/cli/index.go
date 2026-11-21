package cli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"time"
)

var indexIDPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)
var waitIndexPoll = func(ctx context.Context) error {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func newIndexID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
func indexJob(value map[string]any) (map[string]any, error) {
	job, ok := value["index"].(map[string]any)
	if !ok {
		return nil, errors.New("Cloud did not return an index operation")
	}
	id, _ := job["id"].(string)
	state, _ := job["status"].(string)
	if !indexIDPattern.MatchString(id) || (state != "queued" && state != "running" && state != "completed" && state != "failed") {
		return nil, errors.New("invalid index operation response")
	}
	return job, nil
}
func indexPending(job map[string]any) bool {
	return job["status"] == "queued" || job["status"] == "running"
}
func indexNumber(v any) string {
	if n, ok := v.(float64); ok && !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 && n <= 9007199254740991 {
		return fmt.Sprintf("%.0f", n)
	}
	return "0"
}
func indexProgress(job map[string]any) string {
	p, _ := job["progress"].(map[string]any)
	scan, _ := p["scan"].(map[string]any)
	stage, _ := p["stage"].(string)
	if len(stage) > 60 || strings.ContainsAny(stage, "\r\n\x1b") {
		stage = "working"
	}
	if stage == "" {
		stage = fmt.Sprint(job["status"])
	}
	return fmt.Sprintf("%s · files %s/%s · chunks %s · embedded %s · reused %s", terminalText(stage), indexNumber(scan["processed"]), indexNumber(scan["total"]), indexNumber(p["chunks"]), indexNumber(p["embedded"]), indexNumber(p["reused"]))
}
func indexOutput(out io.Writer, value map[string]any, format string) error {
	if format == "json" {
		return output(out, value, format)
	}
	if value["index"] == nil {
		_, err := fmt.Fprintln(out, "No Cloud index operation exists for this repository.")
		return err
	}
	job, err := indexJob(value)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Repository: %s\nIndex: %s\nStatus: %s\n%s\n", terminalText(fmt.Sprint(value["repository"])), job["id"], job["status"], indexProgress(job))
	p, _ := job["progress"].(map[string]any)
	if revision, ok := p["revision"].(string); ok {
		fmt.Fprintln(out, "Revision:", terminalText(revision))
	}
	if p["partial"] == true {
		fmt.Fprintln(out, "Index is partial; some files were omitted.")
	}
	if message, ok := p["error"].(string); ok {
		if len(message) > 500 {
			message = message[:500]
		}
		fmt.Fprintln(out, "Error:", terminalText(message))
	}
	if freshness, ok := value["freshness"].(map[string]any); ok && freshness["up_to_date"] == true {
		fmt.Fprintln(out, "Cloud index is already up to date.")
	}
	return nil
}

func indexCommand(c *Client, args []string, o options, in io.Reader, out, log io.Writer) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	p, _, _, err := contextScopeContext(ctx, c, o)
	if err != nil {
		return err
	}
	if p.Repository == "" {
		return errors.New("choose one repository: run inside its connected checkout or pass --repo owner/name")
	}
	query := "repository=" + url.QueryEscape(p.Repository) + "&workspace=" + url.QueryEscape(p.Workspace)
	if len(args) == 1 && args[0] == "status" {
		value, err := c.requestContext(ctx, "GET", "/api/cli/index/status?"+query, nil)
		if err != nil {
			return err
		}
		return indexOutput(out, value, o["format"])
	}
	if len(args) != 0 {
		return errors.New("use index or index status")
	}
	timeout, err := integer(o, "timeout", 1800, 1, 86400)
	if err != nil {
		return err
	}
	ctx, cancelDeadline := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancelDeadline()
	id, err := newIndexID()
	if err != nil {
		return err
	}
	stop := startSpinner(log, "Starting Cloud index...", terminalProgress(o, in, out, log))
	value, err := c.requestContext(ctx, "POST", "/api/cli/index/start", map[string]any{"repository": p.Repository, "workspace": p.Workspace, "request_id": id})
	stop()
	if err != nil {
		return fmt.Errorf("%w; check inkspect index status --repo %s before starting again", err, p.Repository)
	}
	job, err := indexJob(value)
	if err != nil {
		return err
	}
	id = job["id"].(string) // May be a previously queued/running operation.
	if o["no-wait"] != "" {
		return indexOutput(out, value, o["format"])
	}
	show := terminalProgress(o, in, out, log)
	last := ""
	if show && indexPending(job) {
		fmt.Fprintf(log, "Following Cloud index %s. Ctrl+C stops waiting; Cloud indexing continues.\n", id)
	}
	for indexPending(job) {
		if current := indexProgress(job); show && current != last {
			fmt.Fprintln(log, current)
			last = current
		}
		if err = waitIndexPoll(ctx); err == nil {
			var next map[string]any
			next, err = c.requestContext(ctx, "GET", "/api/cli/index/status?"+query+"&id="+url.QueryEscape(id), nil)
			if err == nil {
				var nextJob map[string]any
				nextJob, err = indexJob(next)
				if err == nil && nextJob["id"] != id {
					err = errors.New("Cloud returned a different index operation")
				}
				if err == nil {
					value, job = next, nextJob
				}
			}
		}
		if err != nil {
			_ = indexOutput(out, value, o["format"])
			return fmt.Errorf("index tracking stopped: %w; Cloud work was not cancelled. Check inkspect index status --repo %s", err, p.Repository)
		}
	}
	if err = indexOutput(out, value, o["format"]); err != nil {
		return err
	}
	if job["status"] == "failed" {
		return errors.New("Cloud indexing failed; see the index status above")
	}
	progress, _ := job["progress"].(map[string]any)
	if progress["indexed"] != true {
		return errors.New("index operation completed without a published code index")
	}
	return nil
}
