package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerRunnerPolicyDoesNotMountDockerSocketOrUseShell(t *testing.T) {
	runner := DockerRunner{policy: DefaultPolicy(BackendDocker)}
	args := runner.DockerArgs("/tmp/qsm-room", "/workspace/product", []string{"node", "--check", "app.js"})
	joined := strings.Join(args, "\x00")
	for _, forbidden := range []string{"/var/run/docker.sock", "docker.sock", "\x00sh\x00", "\x00bash\x00", "\x00-c\x00"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("docker args contain forbidden token %q: %#v", forbidden, args)
		}
	}
	for _, want := range []string{"--network\x00none", "--cap-drop\x00ALL", "--security-opt\x00no-new-privileges", "--pids-limit", "--memory", "--cpus", "--volume\x00/tmp/qsm-room:/workspace:rw"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("docker args missing %q: %#v", want, args)
		}
	}
	if !strings.Contains(joined, "--tmpfs\x00/tmp:rw,nosuid,exec,size=512m") {
		t.Fatalf("docker tmpfs must allow private test binary execution while keeping host mount isolated: %#v", args)
	}
}

func TestDockerRunnerMapsRoomLocalAbsoluteArgs(t *testing.T) {
	room := filepath.Join(t.TempDir(), "room")
	script := filepath.Join(room, ".qsm_test", "hidden", "static-web-smoke.cjs")
	product := filepath.Join(room, "product", "index.html")
	args := DockerRunner{policy: DefaultPolicy(BackendDocker)}.DockerArgs(room, "/workspace/product", []string{"node", script, product, "/tmp/outside.js"})
	joined := strings.Join(args, "\x00")
	if !strings.Contains(joined, "/workspace/.qsm_test/hidden/static-web-smoke.cjs") {
		t.Fatalf("expected hidden script path mapped into workspace: %#v", args)
	}
	if !strings.Contains(joined, "/workspace/product/index.html") {
		t.Fatalf("expected product path mapped into workspace: %#v", args)
	}
	if !strings.Contains(joined, "/tmp/outside.js") {
		t.Fatalf("outside absolute paths should remain unchanged for executables/system files: %#v", args)
	}
}

func TestDockerRunnerPassesCommandEnvironmentIntoContainer(t *testing.T) {
	args := DockerRunner{policy: DefaultPolicy(BackendDocker)}.DockerArgs("/tmp/qsm-room", "/workspace", []string{"node", "-e", "console.log(process.env.QSM_EXPECT_FILES)"}, "QSM_EXPECT_FILES=1000", "", "bad-env")
	joined := strings.Join(args, "\x00")
	if !strings.Contains(joined, "--env\x00QSM_EXPECT_FILES=1000") {
		t.Fatalf("expected command env propagated into docker args: %#v", args)
	}
	if strings.Contains(joined, "\x00bad-env\x00") {
		t.Fatalf("malformed env without '=' should not be propagated: %#v", args)
	}
}

func TestInspectSeparatesDockerCLIFromDaemon(t *testing.T) {
	dir := t.TempDir()
	fakeDocker := filepath.Join(dir, "docker")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "Docker version fake"
  exit 0
fi
if [ "$1" = "info" ]; then
  echo "daemon unavailable" >&2
  exit 1
fi
exit 1
`
	if err := os.WriteFile(fakeDocker, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	report := Inspect(t.TempDir())
	if !report.Docker.Available {
		t.Fatalf("expected fake Docker CLI available: %#v", report.Docker)
	}
	if report.DockerDaemon.Available {
		t.Fatalf("expected Docker daemon unavailable: %#v", report.DockerDaemon)
	}
	if report.ReadinessLevel != "docker-cli-only" {
		t.Fatalf("expected docker-cli-only readiness, got %q", report.ReadinessLevel)
	}
	if report.HardSandboxReady {
		t.Fatal("CLI-only Docker must not be hard sandbox ready")
	}
}

func TestDockerProbeDoesNotRunChecksWhenDaemonMissing(t *testing.T) {
	dir := t.TempDir()
	fakeDocker := filepath.Join(dir, "docker")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "Docker version fake"
  exit 0
fi
if [ "$1" = "info" ]; then
  echo "daemon unavailable" >&2
  exit 1
fi
exit 1
`
	if err := os.WriteFile(fakeDocker, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	report := Probe(t.TempDir(), BackendDocker)
	if !report.Attempted || report.Valid || !report.InfraUnavailable {
		t.Fatalf("expected infra-unavailable probe without semantic checks: %#v", report)
	}
	if report.InsideReadPassed || report.OutsideReadBlocked || report.NetworkBlocked || report.TimeoutKilled {
		t.Fatalf("daemon-missing probe must not claim safety checks: %#v", report)
	}
}
