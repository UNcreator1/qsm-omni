package capacity

import (
	"fmt"
	"math"
	"runtime"
)

const MiB int64 = 1024 * 1024

type Hardware struct {
	MemoryBytes int64 `json:"memory_bytes"`
	LogicalCPU  int   `json:"logical_cpu"`
}

type NodeProfile struct {
	RouterSocketMiB  int64   `json:"router_socket_mib"`
	OpenCodeCliMiB   int64   `json:"opencode_cli_mib"`
	HarnessMiB       int64   `json:"harness_mib"`
	WikiWorkingMiB   int64   `json:"wiki_working_mib"`
	PerNodeCPUWeight float64 `json:"per_node_cpu_weight"`
}

type Plan struct {
	Hardware             Hardware    `json:"hardware"`
	Profile              NodeProfile `json:"profile"`
	ReservedSystemMiB    int64       `json:"reserved_system_mib"`
	ReservedSharedMiB    int64       `json:"reserved_shared_mib"`
	PerNodeMiB           int64       `json:"per_node_mib"`
	MemoryLimitedNodes   int         `json:"memory_limited_nodes"`
	CPULimitedNodes      int         `json:"cpu_limited_nodes"`
	RecommendedNodes     int         `json:"recommended_nodes"`
	RecommendedPositions int         `json:"recommended_positions"`
	Notes                []string    `json:"notes"`
}

func DefaultProfile() NodeProfile {
	return NodeProfile{
		RouterSocketMiB:  64,
		OpenCodeCliMiB:   384,
		HarnessMiB:       256,
		WikiWorkingMiB:   128,
		PerNodeCPUWeight: 1.25,
	}
}

func LocalHardware() Hardware {
	return Hardware{
		MemoryBytes: detectMemoryBytes(),
		LogicalCPU:  runtime.NumCPU(),
	}
}

func Estimate(hw Hardware, profile NodeProfile) Plan {
	if hw.MemoryBytes <= 0 {
		hw.MemoryBytes = 8 * 1024 * MiB
	}
	if hw.LogicalCPU <= 0 {
		hw.LogicalCPU = 1
	}
	if profile.PerNodeCPUWeight <= 0 {
		profile = DefaultProfile()
	}
	totalMiB := hw.MemoryBytes / MiB
	reservedSystem := maxInt64(2048, totalMiB/4)
	reservedShared := int64(1536)
	perNode := profile.RouterSocketMiB + profile.OpenCodeCliMiB + profile.HarnessMiB + profile.WikiWorkingMiB
	availableMiB := totalMiB - reservedSystem - reservedShared
	memNodes := int(availableMiB / perNode)
	if memNodes < 1 {
		memNodes = 1
	}
	cpuNodes := int(math.Floor(float64(hw.LogicalCPU-1) / profile.PerNodeCPUWeight))
	if cpuNodes < 1 {
		cpuNodes = 1
	}
	recommended := minInt(memNodes, cpuNodes)
	if recommended < 1 {
		recommended = 1
	}
	notes := []string{
		"9Router API calls are mostly network-bound but still need socket buffers and result memory.",
		"OpenCode CLI is modeled as the dominant per-node local process cost.",
		"Harness agents are modeled as local Plan-Code-Test-Verify supervisors.",
		"Wiki LLM memory is shared conceptually, with a small per-node working context budget.",
	}
	return Plan{
		Hardware:             hw,
		Profile:              profile,
		ReservedSystemMiB:    reservedSystem,
		ReservedSharedMiB:    reservedShared,
		PerNodeMiB:           perNode,
		MemoryLimitedNodes:   memNodes,
		CPULimitedNodes:      cpuNodes,
		RecommendedNodes:     recommended,
		RecommendedPositions: recommended,
		Notes:                notes,
	}
}

func (p Plan) Summary() string {
	return fmt.Sprintf("hardware=%dMiB/%dCPU per_node=%dMiB memory_cap=%d cpu_cap=%d recommended_nodes=%d",
		p.Hardware.MemoryBytes/MiB, p.Hardware.LogicalCPU, p.PerNodeMiB, p.MemoryLimitedNodes, p.CPULimitedNodes, p.RecommendedNodes)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
