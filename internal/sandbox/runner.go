package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	BackendRoom   = "room"
	BackendDocker = "docker"
	BackendAuto   = "auto"
)

type Policy struct {
	Backend     string `json:"backend"`
	Image       string `json:"image,omitempty"`
	Network     string `json:"network"`
	CPUs        string `json:"cpus"`
	Memory      string `json:"memory"`
	PidsLimit   int    `json:"pids_limit"`
	ReadOnly    bool   `json:"read_only_root"`
	DropCaps    bool   `json:"drop_capabilities"`
	User        string `json:"user"`
	WorkRoot    string `json:"work_root"`
	Description string `json:"description,omitempty"`
}

type Command struct {
	Name       string
	Cmd        []string
	CWD        string
	Room       string
	Timeout    time.Duration
	Env        []string
	StdoutPath string
	StderrPath string
}

type Result struct {
	Backend    string
	ExitCode   int
	DurationMS int64
	Stdout     string
	Stderr     string
	Error      string
}

type Runner interface {
	Backend() string
	Policy() Policy
	Run(context.Context, Command) Result
}

func DefaultPolicy(backend string) Policy {
	backend = NormalizeBackend(backend)
	return Policy{
		Backend:   backend,
		Image:     getenvDefault("QSM_SANDBOX_DOCKER_IMAGE", "node:22-bookworm"),
		Network:   getenvDefault("QSM_SANDBOX_NETWORK", "none"),
		CPUs:      getenvDefault("QSM_SANDBOX_CPUS", "2"),
		Memory:    getenvDefault("QSM_SANDBOX_MEMORY", "1536m"),
		PidsLimit: envInt("QSM_SANDBOX_PIDS_LIMIT", 256),
		ReadOnly:  envBoolDefault("QSM_SANDBOX_READ_ONLY_ROOT", true),
		DropCaps:  envBoolDefault("QSM_SANDBOX_DROP_CAPS", true),
		User:      getenvDefault("QSM_SANDBOX_USER", "1000:1000"),
		WorkRoot:  "/workspace",
	}
}

func NormalizeBackend(backend string) string {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case BackendDocker:
		return BackendDocker
	case BackendAuto:
		return BackendAuto
	default:
		return BackendRoom
	}
}

func ResolveBackend(backend string) string {
	backend = NormalizeBackend(backend)
	if backend != BackendAuto {
		return backend
	}
	if DockerDaemonAvailable() {
		return BackendDocker
	}
	return BackendRoom
}

func NewRunner(backend string) Runner {
	resolved := ResolveBackend(backend)
	policy := DefaultPolicy(resolved)
	if resolved == BackendDocker {
		return DockerRunner{policy: policy}
	}
	return RoomRunner{policy: policy}
}

type RoomRunner struct {
	policy Policy
}

func (r RoomRunner) Backend() string { return BackendRoom }
func (r RoomRunner) Policy() Policy  { return r.policy }

func (r RoomRunner) Run(parent context.Context, spec Command) Result {
	start := time.Now()
	result := Result{Backend: BackendRoom, ExitCode: -1}
	if len(spec.Cmd) == 0 {
		result.Error = "empty command"
		return result
	}
	if spec.Timeout <= 0 {
		spec.Timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, spec.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, spec.Cmd[0], spec.Cmd[1:]...)
	cmd.Dir = spec.CWD
	cmd.Env = append(os.Environ(), spec.Env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result.DurationMS = time.Since(start).Milliseconds()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	if ctx.Err() == context.DeadlineExceeded {
		result.Error = "command timed out"
		return result
	}
	if err != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Error = err.Error()
		}
	} else {
		result.ExitCode = 0
	}
	writeCommandLogs(spec, result)
	return result
}

type DockerRunner struct {
	policy Policy
}

func (r DockerRunner) Backend() string { return BackendDocker }
func (r DockerRunner) Policy() Policy  { return r.policy }

func (r DockerRunner) Run(parent context.Context, spec Command) Result {
	start := time.Now()
	result := Result{Backend: BackendDocker, ExitCode: -1}
	if len(spec.Cmd) == 0 {
		result.Error = "empty command"
		return result
	}
	if !DockerDaemonAvailable() {
		result.Error = "docker daemon is not reachable"
		return result
	}
	if spec.Timeout <= 0 {
		spec.Timeout = 60 * time.Second
	}
	roomAbs, err := filepath.Abs(spec.Room)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	cwdAbs, err := filepath.Abs(spec.CWD)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	workdir, err := dockerWorkdir(roomAbs, cwdAbs, r.policy.WorkRoot)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	args := r.DockerArgs(roomAbs, workdir, spec.Cmd, spec.Env...)
	ctx, cancel := context.WithTimeout(parent, spec.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = append(os.Environ(), spec.Env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	result.DurationMS = time.Since(start).Milliseconds()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	if ctx.Err() == context.DeadlineExceeded {
		result.Error = "command timed out"
		return result
	}
	if err != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Error = err.Error()
		}
	} else {
		result.ExitCode = 0
	}
	writeCommandLogs(spec, result)
	return result
}

func (r DockerRunner) DockerArgs(room, workdir string, cmd []string, env ...string) []string {
	p := r.policy
	containerCmd := dockerCommandArgs(room, p.WorkRoot, cmd)
	args := []string{
		"run", "--rm",
		"--network", p.Network,
		"--cpus", p.CPUs,
		"--memory", p.Memory,
		"--pids-limit", fmt.Sprint(p.PidsLimit),
		"--user", p.User,
		"--workdir", workdir,
		"--volume", room + ":" + p.WorkRoot + ":rw",
		"--env", "CI=1",
		"--env", "NO_COLOR=1",
		"--env", "HOME=/tmp",
		"--env", "XDG_CACHE_HOME=/tmp/.cache",
		"--env", "GOCACHE=/tmp/go-build",
		"--env", "npm_config_cache=/tmp/npm-cache",
		"--env", "PYTHONDONTWRITEBYTECODE=1",
	}
	for _, value := range env {
		value = strings.TrimSpace(value)
		if value == "" || strings.Contains(value, "\x00") || !strings.Contains(value, "=") {
			continue
		}
		args = append(args, "--env", value)
	}
	if p.ReadOnly {
		args = append(args, "--read-only", "--tmpfs", "/tmp:rw,nosuid,exec,size=512m")
	}
	if p.DropCaps {
		args = append(args, "--cap-drop", "ALL", "--security-opt", "no-new-privileges")
	}
	args = append(args, p.Image)
	args = append(args, containerCmd...)
	return args
}

func dockerCommandArgs(room, workRoot string, cmd []string) []string {
	out := make([]string, 0, len(cmd))
	roomAbs, err := filepath.Abs(room)
	if err != nil {
		roomAbs = room
	}
	for _, arg := range cmd {
		if mapped, ok := dockerMapRoomPath(roomAbs, workRoot, arg); ok {
			out = append(out, mapped)
			continue
		}
		out = append(out, arg)
	}
	return out
}

func dockerMapRoomPath(roomAbs, workRoot, arg string) (string, bool) {
	if !filepath.IsAbs(arg) {
		return "", false
	}
	argAbs, err := filepath.Abs(arg)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(roomAbs, argAbs)
	if err != nil {
		return "", false
	}
	if rel == "." {
		return workRoot, true
	}
	if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return "", false
	}
	return filepath.ToSlash(filepath.Join(workRoot, rel)), true
}

func DockerDaemonAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
	return cmd.Run() == nil
}

func dockerWorkdir(room, cwd, workRoot string) (string, error) {
	rel, err := filepath.Rel(room, cwd)
	if err != nil || rel == "." {
		return workRoot, nil
	}
	if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return "", fmt.Errorf("cwd escapes sandbox room: %s", cwd)
	}
	return filepath.ToSlash(filepath.Join(workRoot, rel)), nil
}

func writeCommandLogs(spec Command, result Result) {
	if spec.StdoutPath != "" {
		_ = os.WriteFile(spec.StdoutPath, []byte(result.Stdout), 0644)
	}
	if spec.StderrPath != "" {
		_ = os.WriteFile(spec.StderrPath, []byte(result.Stderr), 0644)
	}
}

func asExitError(err error, target **exec.ExitError) bool {
	if err == nil {
		return false
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		*target = exitErr
		return true
	}
	return false
}

func getenvDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	var out int
	if _, err := fmt.Sscanf(value, "%d", &out); err != nil || out <= 0 {
		return fallback
	}
	return out
}

func envBoolDefault(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes"
}
