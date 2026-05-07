//go:build darwin

package capacity

import (
	"os/exec"
	"strconv"
	"strings"
)

func detectMemoryBytes() int64 {
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0
	}
	mem, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0
	}
	return mem
}
