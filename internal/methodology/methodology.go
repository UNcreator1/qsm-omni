package methodology

import "strings"

type Phase string

const (
	PhaseSynthesis Phase = "synthesis"
	PhasePlanning  Phase = "planning"
	PhaseBuild     Phase = "build"
	PhaseDebug     Phase = "debug"
	PhaseReview    Phase = "review"
	PhaseCollapse  Phase = "collapse"
)

type Contract struct {
	Name  string
	Phase Phase
	Body  string
}

var contracts = []Contract{
	{
		Name:  "brainstorming",
		Phase: PhaseSynthesis,
		Body: `Brainstorm before implementation:
- State the intended product outcome in one sentence.
- List assumptions separately from verified facts.
- Identify missing context and convert it into testable risks.
- Propose at least one concrete verification path before coding.`,
	},
	{
		Name:  "writing-plans",
		Phase: PhasePlanning,
		Body: `Plan with small executable steps:
- Define the file structure before editing.
- Keep tasks small enough that each can be verified independently.
- Avoid placeholders such as TBD, TODO-later, or "add tests".
- Each task must name the command or product check that proves it is done.`,
	},
	{
		Name:  "test-driven-development",
		Phase: PhaseBuild,
		Body: `Build with test-first discipline when the deliverable has executable behavior:
- Write or declare the expected behavior before implementation.
- Prefer a failing test or minimal runnable probe before production code.
- Run the smallest useful verification after each meaningful change.
- Do not hard-code tests to match the implementation; tests must exercise behavior.`,
	},
	{
		Name:  "verification-before-completion",
		Phase: PhaseBuild,
		Body: `Evidence before completion:
- Do not claim success until a real command or deterministic check has run.
- Record what command/check was run and what it proved.
- If a check is skipped, say why; a skipped check is not a pass.
- QSM will override your claims with its own auto-test report after you finish.`,
	},
	{
		Name:  "systematic-debugging",
		Phase: PhaseDebug,
		Body: `Debug systematically:
- Reproduce the failure first.
- Isolate the smallest failing condition.
- Fix the cause, not the symptom.
- Re-run the original failing check and one neighboring regression check.`,
	},
	{
		Name:  "code-review",
		Phase: PhaseReview,
		Body: `Review against the spec:
- Check for missing requirements before style issues.
- Check security and path isolation boundaries.
- Check that tests prove behavior, not just file existence.
- Report concrete findings with file paths or artifact paths.`,
	},
	{
		Name:  "collapse-readiness",
		Phase: PhaseCollapse,
		Body: `Collapse only verified work:
- Prefer branches with deterministic build, lint, test, and product evidence.
- Treat agent-written evidence as advisory until QSM-owned checks pass.
- Preserve failure lessons in cache so later nodes do not repeat them.
- Deliver only the winning success cluster.`,
	},
}

func All() []Contract {
	out := make([]Contract, len(contracts))
	copy(out, contracts)
	return out
}

func ForPhases(phases ...Phase) []Contract {
	if len(phases) == 0 {
		return All()
	}
	want := map[Phase]bool{}
	for _, phase := range phases {
		want[phase] = true
	}
	var out []Contract
	for _, contract := range contracts {
		if want[contract.Phase] {
			out = append(out, contract)
		}
	}
	return out
}

func Prompt(phases ...Phase) string {
	selected := ForPhases(phases...)
	var b strings.Builder
	b.WriteString("QSM methodology contracts:\n")
	for _, contract := range selected {
		b.WriteString("\n## ")
		b.WriteString(contract.Name)
		b.WriteString(" [")
		b.WriteString(string(contract.Phase))
		b.WriteString("]\n")
		b.WriteString(strings.TrimSpace(contract.Body))
		b.WriteString("\n")
	}
	return b.String()
}
