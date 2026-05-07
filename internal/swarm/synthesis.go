package swarm

import (
	"fmt"
	"strings"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
	"github.com/nemoclaws/quantum-swarm-v3/internal/methodology"
	"github.com/nemoclaws/quantum-swarm-v3/internal/requirements"
)

type Synthesizer struct {
	Lake *lake.Lake
}

func (s Synthesizer) BrainDump(obj Objective, agents []Agent) ([]Hypothesis, error) {
	var out []Hypothesis
	for _, agent := range agents {
		h := Hypothesis{
			AgentID: agent.ID,
			Blueprint: fmt.Sprintf("%s proposes a %s-first implementation for: %s",
				agent.ID, agent.Role, obj.Request),
			Assumptions: []string{
				"Objective can be decomposed into independent positions.",
				"Branch rooms can be tested without mutating production.",
			},
			Risks: []string{
				"Ungrounded model assumptions must be verified in Phase 2.",
				"Collapse must reject branches without evidence.",
			},
			TestStrategy: []string{
				"Persist artifacts.",
				"Create isolated rooms.",
				"Score branches with deterministic checks.",
				"Follow QSM methodology contracts for planning, testing, and verification.",
			},
		}
		if len(agent.Strengths) > 0 {
			h.Blueprint += " Strength focus: " + strings.Join(agent.Strengths, ", ") + "."
		}
		out = append(out, h)
		if s.Lake != nil {
			_, err := s.Lake.Put(lake.Artifact{
				Phase:      lake.PhaseSynthesis,
				Kind:       "zero_shot_hypothesis",
				Source:     agent.ID,
				Claim:      h.Blueprint,
				Content:    strings.Join(append(append([]string{}, h.Assumptions...), h.Risks...), "\n"),
				Confidence: 0.45,
				Verified:   false,
				Metadata: map[string]string{
					"objective_id": obj.ID,
					"model":        agent.Model,
					"provider":     agent.Provider,
				},
			})
			if err != nil {
				return nil, err
			}
			_, err = s.Lake.Put(lake.Artifact{
				Phase:      lake.PhaseSynthesis,
				Kind:       "methodology_contract",
				Source:     "qsm-methodology",
				Claim:      "Nodes must use QSM-native methodology contracts instead of ad-hoc self-verification.",
				Content:    methodology.Prompt(methodology.PhaseSynthesis, methodology.PhasePlanning, methodology.PhaseBuild),
				Confidence: 0.9,
				Verified:   true,
				Metadata: map[string]string{
					"objective_id": obj.ID,
				},
			})
			if err != nil {
				return nil, err
			}
			_, err = s.Lake.Put(lake.Artifact{
				Phase:      lake.PhaseSynthesis,
				Kind:       "force_requirements_checklist",
				Source:     "qsm-force-requirements",
				Claim:      "Every build must assess top-tier production-readiness requirements and mark missing evidence as GAP.",
				Content:    requirements.Prompt(),
				Confidence: 0.9,
				Verified:   true,
				Metadata: map[string]string{
					"objective_id": obj.ID,
				},
			})
			if err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}
