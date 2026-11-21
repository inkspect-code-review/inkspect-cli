package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

func terminalProgress(o options, in io.Reader, out, log io.Writer) bool {
	if o["format"] == "json" || os.Getenv("TERM") == "dumb" || os.Getenv("INKSPECT_NO_PROGRESS") == "1" {
		return false
	}
	output, ok := out.(*os.File)
	if !ok {
		return false
	}
	diagnostic, ok := log.(*os.File)
	if !ok {
		return false
	}
	return isTerminal(in) && isTerminal(output) && isTerminal(diagnostic)
}

// Plain carriage-return frames work on Unix and Windows consoles without ANSI
// modes, cursor hiding, or a dependency on a particular terminal emulator.
func startSpinner(w io.Writer, label string, enabled bool) func() {
	if !enabled {
		return func() {}
	}
	done, stopped := make(chan struct{}), make(chan struct{})
	var once sync.Once
	fmt.Fprintf(w, "\r| %s", label)
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		frames := []string{"/", "-", "\\", "|"}
		i := 0
		for {
			select {
			case <-ticker.C:
				fmt.Fprintf(w, "\r%s %s", frames[i%len(frames)], label)
				i++
			case <-done:
				fmt.Fprintf(w, "\r%s\r", strings.Repeat(" ", len(label)+2))
				return
			}
		}
	}()
	return func() { once.Do(func() { close(done); <-stopped }) }
}

func askWithProgress(c *Client, q string, o options, in io.Reader, out, log io.Writer) (map[string]any, error) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	stop := startSpinner(log, "Thinking...", terminalProgress(o, in, out, log))
	defer stop()
	value, err := askContext(ctx, c, q, o)
	if errors.Is(err, context.Canceled) {
		return nil, errors.New("context request cancelled")
	}
	return value, err
}
