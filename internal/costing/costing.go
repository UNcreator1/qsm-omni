package costing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/swarm"
)

const Schema = "qsm.cost_report.v1"

type Report struct {
	Schema          string     `json:"schema"`
	ObjectiveID     string     `json:"objective_id"`
	CreatedAt       time.Time  `json:"created_at"`
	Currency        string     `json:"currency"`
	Estimated       bool       `json:"estimated"`
	RateSource      string     `json:"rate_source"`
	InputTokens     int        `json:"input_tokens"`
	OutputTokens    int        `json:"output_tokens"`
	TotalTokens     int        `json:"total_tokens"`
	EstimatedUSD    float64    `json:"estimated_usd"`
	CostPerSuccess  float64    `json:"cost_per_success_usd,omitempty"`
	ModelSummaries  []ModelSum `json:"model_summaries,omitempty"`
	Nodes           []NodeCost `json:"nodes"`
	Recommendations []string   `json:"recommendations,omitempty"`
}

type ModelSum struct {
	Model        string  `json:"model"`
	Nodes        int     `json:"nodes"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	EstimatedUSD float64 `json:"estimated_usd"`
}

type NodeCost struct {
	PositionID       string  `json:"position_id"`
	AgentID          string  `json:"agent_id,omitempty"`
	Model            string  `json:"model,omitempty"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	EstimatedUSD     float64 `json:"estimated_usd"`
	ObservedUsage    bool    `json:"observed_usage"`
	EstimationMethod string  `json:"estimation_method"`
	Successful       bool    `json:"successful"`
	InputUSDPer1M    float64 `json:"input_usd_per_1m,omitempty"`
	OutputUSDPer1M   float64 `json:"output_usd_per_1m,omitempty"`
}

func Analyze(report swarm.RunReport) Report {
	out := Report{
		Schema:      Schema,
		ObjectiveID: report.ObjectiveID,
		CreatedAt:   time.Now().UTC(),
		Currency:    "USD",
		Estimated:   true,
		RateSource:  "env QSM_COST_USD_PER_1M_INPUT[_MODEL]/OUTPUT[_MODEL]; defaults to 0 when unset",
	}
	models := map[string]*ModelSum{}
	successes := 0
	for _, result := range report.Results {
		node := nodeCost(result)
		out.Nodes = append(out.Nodes, node)
		out.InputTokens += node.InputTokens
		out.OutputTokens += node.OutputTokens
		out.TotalTokens += node.TotalTokens
		out.EstimatedUSD += node.EstimatedUSD
		if node.Successful {
			successes++
		}
		key := node.Model
		if key == "" {
			key = "unknown"
		}
		sum := models[key]
		if sum == nil {
			sum = &ModelSum{Model: key}
			models[key] = sum
		}
		sum.Nodes++
		sum.InputTokens += node.InputTokens
		sum.OutputTokens += node.OutputTokens
		sum.TotalTokens += node.TotalTokens
		sum.EstimatedUSD += node.EstimatedUSD
	}
	if successes > 0 {
		out.CostPerSuccess = out.EstimatedUSD / float64(successes)
	}
	for _, sum := range models {
		out.ModelSummaries = append(out.ModelSummaries, *sum)
	}
	sort.Slice(out.ModelSummaries, func(i, j int) bool { return out.ModelSummaries[i].Model < out.ModelSummaries[j].Model })
	sort.Slice(out.Nodes, func(i, j int) bool { return out.Nodes[i].PositionID < out.Nodes[j].PositionID })
	if out.EstimatedUSD == 0 {
		out.Recommendations = append(out.Recommendations, "Configure model price env vars to turn token estimates into dollar accounting.")
	}
	if out.TotalTokens == 0 {
		out.Recommendations = append(out.Recommendations, "No token evidence found; verify harness logs and provider usage metadata capture.")
	}
	return out
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
	if err := os.WriteFile(filepath.Join(stateDir, "cost_report.json"), data, 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir, "cost_report.md"), []byte(Markdown(report)), 0644)
}

func Markdown(report Report) string {
	var b strings.Builder
	b.WriteString("# QSM Cost Report\n\n")
	b.WriteString(fmt.Sprintf("- Objective: `%s`\n", report.ObjectiveID))
	b.WriteString(fmt.Sprintf("- Estimated: `%v`\n", report.Estimated))
	b.WriteString(fmt.Sprintf("- Input tokens: `%d`\n", report.InputTokens))
	b.WriteString(fmt.Sprintf("- Output tokens: `%d`\n", report.OutputTokens))
	b.WriteString(fmt.Sprintf("- Total tokens: `%d`\n", report.TotalTokens))
	b.WriteString(fmt.Sprintf("- Estimated cost: `$%.6f`\n", report.EstimatedUSD))
	if report.CostPerSuccess > 0 {
		b.WriteString(fmt.Sprintf("- Cost per successful node: `$%.6f`\n", report.CostPerSuccess))
	}
	b.WriteString("\n## Nodes\n\n")
	b.WriteString("| Position | Model | In | Out | Total | Cost | Method |\n")
	b.WriteString("| --- | --- | ---: | ---: | ---: | ---: | --- |\n")
	for _, node := range report.Nodes {
		b.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %d | %.6f | %s |\n", node.PositionID, node.Model, node.InputTokens, node.OutputTokens, node.TotalTokens, node.EstimatedUSD, node.EstimationMethod))
	}
	if len(report.Recommendations) > 0 {
		b.WriteString("\n## Recommendations\n\n")
		for _, rec := range report.Recommendations {
			b.WriteString("- " + rec + "\n")
		}
	}
	return b.String()
}

func nodeCost(result swarm.BranchResult) NodeCost {
	model := result.AgentModel
	if model == "" {
		model = "unknown"
	}
	input, output, observed := usageFromMetadata(result.Metadata)
	method := "provider_usage_metadata"
	if input == 0 && output == 0 {
		input = estimateTokensFromFile(filepath.Join(result.Room, ".qsm_memory", "AGENTS.md")) + estimateTokensFromFile(filepath.Join(result.Room, ".qsm_memory", "CACHE.md"))
		output = estimateTokensFromFile(filepath.Join(result.Room, "deepagents.events.jsonl")) + estimateTokensFromFile(result.EvidencePath)
		method = "estimated_chars_div_4"
	}
	inRate := rateFor(model, "INPUT")
	outRate := rateFor(model, "OUTPUT")
	cost := (float64(input)/1_000_000.0)*inRate + (float64(output)/1_000_000.0)*outRate
	return NodeCost{
		PositionID:       result.PositionID,
		AgentID:          result.AgentID,
		Model:            model,
		InputTokens:      input,
		OutputTokens:     output,
		TotalTokens:      input + output,
		EstimatedUSD:     cost,
		ObservedUsage:    observed,
		EstimationMethod: method,
		Successful:       result.BuildPassed && result.TestPassed && result.LintPassed && result.Error == "",
		InputUSDPer1M:    inRate,
		OutputUSDPer1M:   outRate,
	}
}

func usageFromMetadata(meta map[string]any) (int, int, bool) {
	if len(meta) == 0 {
		return 0, 0, false
	}
	input := anyInt(meta["input_tokens"]) + anyInt(meta["prompt_tokens"])
	output := anyInt(meta["output_tokens"]) + anyInt(meta["completion_tokens"])
	return input, output, input > 0 || output > 0
}

func anyInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(typed))
		return n
	default:
		return 0
	}
}

func estimateTokensFromFile(path string) int {
	if strings.TrimSpace(path) == "" {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return len(data) / 4
}

func rateFor(model, direction string) float64 {
	suffix := sanitizeModel(model)
	for _, key := range []string{
		"QSM_COST_USD_PER_1M_" + direction + "_" + suffix,
		"QSM_COST_USD_PER_1M_" + direction,
	} {
		if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
			if value, err := strconv.ParseFloat(raw, 64); err == nil && value >= 0 {
				return value
			}
		}
	}
	return 0
}

func sanitizeModel(model string) string {
	model = strings.ToUpper(model)
	replacer := strings.NewReplacer("/", "_", "-", "_", ".", "_", ":", "_")
	return replacer.Replace(model)
}
