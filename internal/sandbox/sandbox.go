package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const Schema = "qsm.sandbox_report.v1"

type ToolStatus struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type Report struct {
	Schema             string      `json:"schema"`
	Root               string      `json:"root"`
	OS                 string      `json:"os"`
	CreatedAt          time.Time   `json:"created_at"`
	CurrentIsolation   string      `json:"current_isolation"`
	ReadinessLevel     string      `json:"readiness_level"`
	HardSandboxReady   bool        `json:"hard_sandbox_ready"`
	MicroVMRecommended bool        `json:"microvm_recommended"`
	CLIAvailable       bool        `json:"cli_available"`
	DaemonReachable    bool        `json:"daemon_reachable"`
	ImageAvailable     bool        `json:"image_available"`
	ProbeAttempted     bool        `json:"probe_attempted"`
	ProbeValid         bool        `json:"probe_valid"`
	Docker             ToolStatus  `json:"docker"`
	DockerDaemon       ToolStatus  `json:"docker_daemon"`
	DockerImage        ToolStatus  `json:"docker_image"`
	DockerInfo         string      `json:"docker_info,omitempty"`
	DockerSecurity     []string    `json:"docker_security,omitempty"`
	DockerSandboxCLI   ToolStatus  `json:"docker_sandbox_cli"`
	MacOSSandboxExec   ToolStatus  `json:"macos_sandbox_exec"`
	Tmux               ToolStatus  `json:"tmux"`
	Probe              ProbeReport `json:"probe,omitempty"`
	Policy             Policy      `json:"policy,omitempty"`
	Findings           []string    `json:"findings,omitempty"`
	Recommendations    []string    `json:"recommendations,omitempty"`
}

type ProbeReport struct {
	Backend            string    `json:"backend,omitempty"`
	Passed             bool      `json:"passed"`
	Attempted          bool      `json:"attempted"`
	Valid              bool      `json:"valid"`
	InfraUnavailable   bool      `json:"infra_unavailable,omitempty"`
	InsideReadPassed   bool      `json:"inside_read_passed"`
	OutsideReadBlocked bool      `json:"outside_read_blocked"`
	NetworkBlocked     bool      `json:"network_blocked"`
	TimeoutKilled      bool      `json:"timeout_killed"`
	Error              string    `json:"error,omitempty"`
	CreatedAt          time.Time `json:"created_at,omitempty"`
}

func Inspect(root string) Report {
	absRoot, _ := filepath.Abs(root)
	report := Report{
		Schema:             Schema,
		Root:               absRoot,
		OS:                 runtime.GOOS,
		CreatedAt:          time.Now().UTC(),
		CurrentIsolation:   "local room directory isolation",
		ReadinessLevel:     "room-only",
		MicroVMRecommended: true,
		Docker:             tool("docker", "--version"),
		DockerDaemon:       dockerDaemonStatus(),
		DockerSandboxCLI:   tool("sbx", "--version"),
		MacOSSandboxExec:   tool("sandbox-exec", "-h"),
		Tmux:               tool("tmux", "-V"),
		Policy:             DefaultPolicy(BackendRoom),
	}
	report.CLIAvailable = report.Docker.Available
	report.DaemonReachable = report.DockerDaemon.Available
	if report.Docker.Available && report.DockerDaemon.Available {
		info, security := dockerInfo()
		report.DockerInfo = info
		report.DockerSecurity = security
		report.ReadinessLevel = "container-capable"
		report.Policy = DefaultPolicy(BackendDocker)
		report.DockerImage = dockerImageStatus(report.Policy.Image)
		report.ImageAvailable = report.DockerImage.Available
		report.Findings = append(report.Findings, "Docker CLI and daemon are available for container-backed room execution; QSM still requires a sandbox probe before claiming hard readiness.")
		if !report.ImageAvailable {
			report.Findings = append(report.Findings, "Docker sandbox image is not available locally: "+report.Policy.Image)
		}
	} else if report.Docker.Available {
		report.ReadinessLevel = "docker-cli-only"
		report.DockerImage = ToolStatus{Name: "docker-image", Available: false, Detail: "Docker daemon unavailable; image probe skipped"}
		report.Findings = append(report.Findings, "Docker CLI is available but the daemon is not reachable; QSM must not claim container-capable execution.")
	} else {
		report.DockerImage = ToolStatus{Name: "docker-image", Available: false, Detail: "Docker CLI unavailable; image probe skipped"}
		report.Findings = append(report.Findings, "Docker is not available; QSM currently falls back to local room isolation only.")
	}
	if report.DockerSandboxCLI.Available {
		report.ReadinessLevel = "microvm-capable"
		report.HardSandboxReady = true
		report.Findings = append(report.Findings, "Docker Sandbox CLI is available; this is the preferred local hard-isolation target for coding agents.")
	}
	if runtime.GOOS == "darwin" && report.MacOSSandboxExec.Available {
		report.Findings = append(report.Findings, "macOS sandbox-exec is present, but should be treated as a best-effort compatibility layer, not the primary production isolation boundary.")
	}
	if report.Tmux.Available {
		report.Findings = append(report.Findings, "tmux is available for persistent terminal/session management similar to Agent of Empires style workflows.")
	}
	report.Recommendations = sandboxRecommendations(report)
	return report
}

func InspectWithProbe(root, backend string) Report {
	report := Inspect(root)
	probe := Probe(root, backend)
	report.Probe = probe
	report.ProbeAttempted = probe.Attempted
	report.ProbeValid = probe.Valid
	report.Policy = DefaultPolicy(probe.Backend)
	if probe.Backend == BackendDocker && probe.Passed {
		report.ReadinessLevel = "docker-probed"
		report.HardSandboxReady = true
		report.CurrentIsolation = "docker command sandbox"
		report.Findings = append(report.Findings, "Docker sandbox probe passed: inside-room access worked and outside-room access was blocked.")
	} else if probe.Backend == BackendDocker && !probe.Passed {
		report.HardSandboxReady = false
		report.Findings = append(report.Findings, "Docker sandbox probe failed: "+probe.Error)
	} else if probe.Backend == BackendRoom {
		report.HardSandboxReady = false
		report.Findings = append(report.Findings, "Room-only probe is informational and does not satisfy production hard sandbox requirements.")
	}
	report.Recommendations = sandboxRecommendations(report)
	return report
}

func Write(root string, report Report) error {
	stateDir := filepath.Join(root, ".state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stateDir, "sandbox_report.json"), data, 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir, "sandbox_report.md"), []byte(Markdown(report)), 0644)
}

func Markdown(report Report) string {
	var b strings.Builder
	b.WriteString("# QSM Sandbox Readiness Report\n\n")
	fmt.Fprintf(&b, "- Root: `%s`\n", report.Root)
	fmt.Fprintf(&b, "- OS: `%s`\n", report.OS)
	fmt.Fprintf(&b, "- Current isolation: `%s`\n", report.CurrentIsolation)
	fmt.Fprintf(&b, "- Readiness: `%s`\n", report.ReadinessLevel)
	fmt.Fprintf(&b, "- Hard sandbox ready: `%v`\n", report.HardSandboxReady)
	fmt.Fprintf(&b, "- MicroVM recommended: `%v`\n", report.MicroVMRecommended)
	if report.Probe.Backend != "" {
		fmt.Fprintf(&b, "- Probe: backend=`%s` passed=`%v` inside=`%v` outside_blocked=`%v` network_blocked=`%v` timeout_killed=`%v`\n",
			report.Probe.Backend, report.Probe.Passed, report.Probe.InsideReadPassed, report.Probe.OutsideReadBlocked, report.Probe.NetworkBlocked, report.Probe.TimeoutKilled)
	}
	if report.Policy.Backend != "" {
		fmt.Fprintf(&b, "- Policy: backend=`%s` network=`%s` cpus=`%s` memory=`%s` pids=`%d`\n",
			report.Policy.Backend, report.Policy.Network, report.Policy.CPUs, report.Policy.Memory, report.Policy.PidsLimit)
	}
	b.WriteString("\n## Tools\n\n")
	for _, tool := range []ToolStatus{report.Docker, report.DockerDaemon, report.DockerSandboxCLI, report.MacOSSandboxExec, report.Tmux} {
		status := "missing"
		if tool.Available {
			status = "available"
		}
		fmt.Fprintf(&b, "- `%s`: %s", tool.Name, status)
		if tool.Path != "" {
			fmt.Fprintf(&b, " at `%s`", tool.Path)
		}
		if tool.Detail != "" {
			fmt.Fprintf(&b, " (%s)", oneLine(tool.Detail))
		}
		b.WriteString("\n")
	}
	if report.DockerImage.Name != "" {
		status := "missing"
		if report.DockerImage.Available {
			status = "available"
		}
		fmt.Fprintf(&b, "- `%s`: %s", report.DockerImage.Name, status)
		if report.DockerImage.Detail != "" {
			fmt.Fprintf(&b, " (%s)", oneLine(report.DockerImage.Detail))
		}
		b.WriteString("\n")
	}
	if len(report.DockerSecurity) > 0 {
		b.WriteString("\n## Docker Security Options\n\n")
		for _, item := range report.DockerSecurity {
			b.WriteString("- " + item + "\n")
		}
	}
	if len(report.Findings) > 0 {
		b.WriteString("\n## Findings\n\n")
		for _, finding := range report.Findings {
			b.WriteString("- " + finding + "\n")
		}
	}
	if len(report.Recommendations) > 0 {
		b.WriteString("\n## Recommendations\n\n")
		for _, rec := range report.Recommendations {
			b.WriteString("- " + rec + "\n")
		}
	}
	return b.String()
}

func tool(name string, args ...string) ToolStatus {
	path, err := exec.LookPath(name)
	if err != nil {
		return ToolStatus{Name: name, Available: false, Detail: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()
	detail := strings.TrimSpace(out.String())
	if err != nil && detail == "" {
		detail = err.Error()
	}
	return ToolStatus{Name: name, Available: true, Path: path, Detail: detail}
}

func dockerInfo() (string, []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "info", "--format", "{{json .SecurityOptions}}")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(out.String()), nil
	}
	var security []string
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &security); err != nil {
		return strings.TrimSpace(out.String()), nil
	}
	return strings.TrimSpace(out.String()), security
}

func dockerDaemonStatus() ToolStatus {
	if _, err := exec.LookPath("docker"); err != nil {
		return ToolStatus{Name: "docker-daemon", Available: false, Detail: "docker CLI is not available"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	detail := strings.TrimSpace(out.String())
	if err != nil {
		if detail == "" {
			detail = err.Error()
		}
		return ToolStatus{Name: "docker-daemon", Available: false, Detail: detail}
	}
	return ToolStatus{Name: "docker-daemon", Available: true, Detail: detail}
}

func dockerImageStatus(image string) ToolStatus {
	if strings.TrimSpace(image) == "" {
		return ToolStatus{Name: "docker-image", Available: false, Detail: "empty image"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	detail := strings.TrimSpace(out.String())
	if err != nil {
		if detail == "" {
			detail = err.Error()
		}
		return ToolStatus{Name: "docker-image", Available: false, Detail: detail}
	}
	return ToolStatus{Name: "docker-image", Available: true, Detail: image}
}

func Probe(root, backend string) ProbeReport {
	backend = ResolveBackend(backend)
	report := ProbeReport{Backend: backend, Attempted: true, CreatedAt: time.Now().UTC()}
	if backend == BackendDocker && !DockerDaemonAvailable() {
		report.InfraUnavailable = true
		report.Error = "docker daemon is not reachable; sandbox probe not executed"
		return report
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	baseProbeRoot := filepath.Join(absRoot, ".state", "sandbox_probe")
	if err := os.MkdirAll(baseProbeRoot, 0755); err != nil {
		report.Error = err.Error()
		return report
	}
	probeRoot, err := os.MkdirTemp(baseProbeRoot, backend+"-")
	if err != nil {
		report.Error = err.Error()
		return report
	}
	room := filepath.Join(probeRoot, "room")
	outside := filepath.Join(probeRoot, "outside-secret.txt")
	if err := os.MkdirAll(room, 0755); err != nil {
		report.Error = err.Error()
		return report
	}
	if err := os.WriteFile(filepath.Join(room, "inside.txt"), []byte("inside-ok\n"), 0644); err != nil {
		report.Error = err.Error()
		return report
	}
	if err := os.WriteFile(outside, []byte("outside-deny\n"), 0644); err != nil {
		report.Error = err.Error()
		return report
	}
	runner := NewRunner(backend)
	ctx := context.Background()
	inside := runner.Run(ctx, Command{Name: "inside read", Room: room, CWD: room, Cmd: []string{"cat", "inside.txt"}, Timeout: 15 * time.Second})
	report.InsideReadPassed = inside.ExitCode == 0 && strings.Contains(inside.Stdout, "inside-ok")
	outsideRead := runner.Run(ctx, Command{Name: "outside read", Room: room, CWD: room, Cmd: []string{"cat", outside}, Timeout: 15 * time.Second})
	report.OutsideReadBlocked = outsideRead.ExitCode != 0 || outsideRead.Error != ""
	network := runner.Run(ctx, Command{Name: "network blocked", Room: room, CWD: room, Cmd: []string{"node", "-e", `require("dns").lookup("example.com",e=>process.exit(e?0:1))`}, Timeout: 15 * time.Second})
	report.NetworkBlocked = network.ExitCode == 0 && network.Error == ""
	timeout := runner.Run(ctx, Command{Name: "timeout", Room: room, CWD: room, Cmd: []string{"node", "-e", `setTimeout(()=>{},10000)`}, Timeout: 500 * time.Millisecond})
	report.TimeoutKilled = strings.Contains(timeout.Error, "timed out")
	if backend == BackendRoom {
		report.Passed = false
		report.Valid = true
		report.Error = "room-only probe cannot block outside-room reads; not a hard sandbox"
		return report
	}
	report.Passed = report.InsideReadPassed && report.OutsideReadBlocked && report.NetworkBlocked && report.TimeoutKilled
	report.Valid = true
	if !report.Passed {
		report.Error = fmt.Sprintf("inside=%v outside_blocked=%v network_blocked=%v timeout=%v", report.InsideReadPassed, report.OutsideReadBlocked, report.NetworkBlocked, report.TimeoutKilled)
	}
	return report
}

func sandboxRecommendations(report Report) []string {
	var out []string
	out = append(out, "Production target: execute each node in a hard sandbox boundary, preferably a microVM or equivalent VM-backed sandbox with isolated network, filesystem, Docker engine, and credential proxy.")
	if report.Docker.Available && report.DockerDaemon.Available {
		out = append(out, "Container fallback: run agents as non-root, drop capabilities, set CPU/memory/pids limits, avoid mounting the host Docker socket, and mount only the room/workspace required for that node.")
		if !report.ImageAvailable {
			out = append(out, "Prepare the sandbox image before production QA: docker pull "+report.Policy.Image+" or set QSM_SANDBOX_DOCKER_IMAGE to a prebuilt QSM image containing Go, Node, Python, pytest, and Playwright dependencies.")
		}
	} else if report.Docker.Available {
		out = append(out, "Start Docker Desktop or Colima before using the Docker sandbox backend; QSM will report daemon-missing rather than auto-starting it.")
	} else {
		out = append(out, "Install Docker or a VM-backed sandbox backend before claiming hard isolation.")
	}
	if report.Tmux.Available {
		out = append(out, "Use tmux-backed persistent sessions for OpenCode terminal management, log replay, and stuck/waiting/idle node detection.")
	} else {
		out = append(out, "Install tmux if you want Agent-of-Empires-style persistent terminal supervision for OpenCode nodes.")
	}
	out = append(out, "Keep SBOM/license/compliance scans as a separate gate; they are important for enterprise release but not blocking this sandbox milestone.")
	return out
}

func oneLine(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 140 {
		return value[:140] + "..."
	}
	return value
}
