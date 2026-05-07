package council

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	Dir     string
	Timeout time.Duration
}

type Advice struct {
	Mode          string    `json:"mode"`
	Prompt        string    `json:"prompt"`
	Attachment    string    `json:"attachment,omitempty"`
	CommandOutput string    `json:"command_output"`
	Grok          string    `json:"grok,omitempty"`
	ChatGPT       string    `json:"chatgpt,omitempty"`
	StreamTail    string    `json:"stream_tail,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at"`
	DurationMS    int64     `json:"duration_ms"`
	Error         string    `json:"error,omitempty"`
}

func (c Client) Ask(ctx context.Context, mode, prompt, attachment string) Advice {
	startedAt := time.Now().UTC()
	advice := Advice{Mode: mode, Prompt: prompt, Attachment: attachment, StartedAt: startedAt}
	if c.Dir == "" {
		c.Dir = "/Users/nexus/Downloads/NemoClaw/Council"
	}
	if c.Timeout <= 0 {
		c.Timeout = 5 * time.Minute
	}
	if mode == "" {
		mode = "status"
		advice.Mode = mode
	}

	runCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	args := []string{filepath.Join(c.Dir, "council.sh"), mode}
	if prompt != "" {
		args = append(args, prompt)
	}
	if attachment != "" {
		args = append(args, attachment)
	}
	cmd := exec.CommandContext(runCtx, "bash", args...)
	cmd.Dir = c.Dir
	out, err := cmd.CombinedOutput()
	advice.CommandOutput = string(out)
	if err != nil {
		advice.Error = err.Error()
		if runCtx.Err() != nil {
			advice.Error = runCtx.Err().Error()
		}
	}
	advice.Grok = readIfExists(filepath.Join(c.Dir, "grok_latest.txt"))
	advice.ChatGPT = readIfExists(filepath.Join(c.Dir, "chatgpt_latest.txt"))
	advice.StreamTail = tail(readIfExists(filepath.Join(c.Dir, "stream.txt")), 12000)
	advice.CompletedAt = time.Now().UTC()
	advice.DurationMS = advice.CompletedAt.Sub(startedAt).Milliseconds()
	return advice
}

func (a Advice) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "mode=%s duration_ms=%d", a.Mode, a.DurationMS)
	if a.Error != "" {
		fmt.Fprintf(&b, " error=%s", a.Error)
	}
	if a.Grok != "" {
		fmt.Fprintf(&b, "\n\n[Grok]\n%s", a.Grok)
	}
	if a.ChatGPT != "" {
		fmt.Fprintf(&b, "\n\n[ChatGPT]\n%s", a.ChatGPT)
	}
	if a.Grok == "" && a.ChatGPT == "" && a.CommandOutput != "" {
		fmt.Fprintf(&b, "\n\n[Command]\n%s", a.CommandOutput)
	}
	return b.String()
}

func readIfExists(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func tail(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[len(value)-max:]
}
