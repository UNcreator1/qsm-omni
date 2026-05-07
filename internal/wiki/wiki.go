package wiki

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
)

type Compiler struct {
	OutDir string
}

func (c Compiler) Compile(artifacts []lake.Artifact) error {
	if c.OutDir == "" {
		return fmt.Errorf("wiki output directory is required")
	}
	if err := os.MkdirAll(c.OutDir, 0755); err != nil {
		return err
	}
	sort.Slice(artifacts, func(i, j int) bool {
		if artifacts[i].Phase == artifacts[j].Phase {
			return artifacts[i].ID < artifacts[j].ID
		}
		return artifacts[i].Phase < artifacts[j].Phase
	})
	var b strings.Builder
	b.WriteString("# Karpathy LLM Wiki\n\n")
	b.WriteString("This file is generated from verified and unverified swarm artifacts. Treat provenance as part of the content.\n\n")
	var grounded []lake.Artifact
	for _, a := range artifacts {
		if a.Kind == "grounded_citation" {
			grounded = append(grounded, a)
		}
		status := "unverified"
		if a.Verified {
			status = "verified"
		}
		b.WriteString(fmt.Sprintf("## %s / %s / %s\n\n", a.Phase, a.Kind, a.Source))
		b.WriteString(fmt.Sprintf("- ID: `%s`\n- Status: `%s`\n- Confidence: `%.2f`\n", a.ID, status, a.Confidence))
		if a.Claim != "" {
			b.WriteString(fmt.Sprintf("- Claim: %s\n", a.Claim))
		}
		if len(a.Contradictions) > 0 {
			b.WriteString("- Contradictions:\n")
			for _, item := range a.Contradictions {
				b.WriteString(fmt.Sprintf("  - %s\n", item))
			}
		}
		b.WriteString("\n")
		if strings.TrimSpace(a.Content) != "" {
			b.WriteString("```text\n")
			b.WriteString(strings.TrimSpace(a.Content))
			b.WriteString("\n```\n\n")
		}
	}
	b.WriteString("## Grounded Citations\n\n")
	if len(grounded) == 0 {
		b.WriteString("No grounded citations have been recorded yet.\n")
	} else {
		for _, a := range grounded {
			source := a.Metadata["source"]
			sourceType := a.Metadata["source_type"]
			sentenceID := a.Metadata["sentence_id"]
			b.WriteString(fmt.Sprintf("- `%s` score=`%.2f` source=`%s` type=`%s` sentence=`%s`\n", a.Source, a.Confidence, source, sourceType, sentenceID))
			if strings.TrimSpace(a.Content) != "" {
				b.WriteString(fmt.Sprintf("  > %s\n", strings.TrimSpace(a.Content)))
			}
		}
	}
	return os.WriteFile(filepath.Join(c.OutDir, "wiki.md"), []byte(b.String()), 0644)
}
