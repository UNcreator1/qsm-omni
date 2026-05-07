package productmanifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const Schema = "qsm_project_manifest.v1"

var supportedKinds = map[string]bool{
	"static-web":     true,
	"cli-tool":       true,
	"go-service":     true,
	"python-package": true,
	"node-fullstack": true,
	"data-transform": true,
}

type Manifest struct {
	Version             string   `json:"version"`
	ProductKind         string   `json:"product_kind"`
	Entrypoints         []string `json:"entrypoints,omitempty"`
	ExpectedArtifacts   []string `json:"expected_artifacts,omitempty"`
	BuildCommands       []string `json:"build_commands,omitempty"`
	TestCommands        []string `json:"test_commands,omitempty"`
	CoverageCommand     string   `json:"coverage_command,omitempty"`
	SmokeCommands       []string `json:"smoke_commands,omitempty"`
	RuntimeRequirements []string `json:"runtime_requirements,omitempty"`
	DeliveryPath        string   `json:"delivery_path,omitempty"`
	MemoryCitations     []string `json:"memory_citations,omitempty"`
}

type ValidationReport struct {
	Schema          string   `json:"schema"`
	ManifestPath    string   `json:"manifest_path,omitempty"`
	ProductPath     string   `json:"product_path"`
	Passed          bool     `json:"passed"`
	ProductKind     string   `json:"product_kind,omitempty"`
	ExpectedChecked []string `json:"expected_checked,omitempty"`
	MemoryCitations []string `json:"memory_citations,omitempty"`
	Errors          []string `json:"errors,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

func SupportedKinds() []string {
	kinds := make([]string, 0, len(supportedKinds))
	for kind := range supportedKinds {
		kinds = append(kinds, kind)
	}
	return kinds
}

func Load(productPath string) (Manifest, string, error) {
	for _, path := range []string{
		filepath.Join(productPath, "qsm_project_manifest.v1.json"),
		filepath.Join(productPath, "qsm_project_manifest.json"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var manifest Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return Manifest{}, path, err
		}
		return manifest, path, nil
	}
	return Manifest{}, "", os.ErrNotExist
}

func Write(path string, manifest Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if manifest.Version == "" {
		manifest.Version = Schema
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func Validate(productPath string) ValidationReport {
	report := ValidationReport{
		Schema:      "qsm.project_manifest_validation.v1",
		ProductPath: productPath,
	}
	manifest, path, err := Load(productPath)
	if err != nil {
		if os.IsNotExist(err) {
			report.Errors = append(report.Errors, "missing qsm_project_manifest.v1.json")
		} else {
			report.Errors = append(report.Errors, "invalid product manifest: "+err.Error())
		}
		return finish(report)
	}
	report.ManifestPath = path
	report.ProductKind = strings.TrimSpace(manifest.ProductKind)
	report.MemoryCitations = append([]string(nil), manifest.MemoryCitations...)
	if manifest.Version != Schema {
		report.Errors = append(report.Errors, fmt.Sprintf("manifest version must be %s", Schema))
	}
	if !supportedKinds[report.ProductKind] {
		report.Errors = append(report.Errors, "unsupported product_kind: "+report.ProductKind)
	}
	if len(manifest.ExpectedArtifacts) == 0 {
		report.Errors = append(report.Errors, "expected_artifacts must not be empty")
	}
	for _, artifact := range manifest.ExpectedArtifacts {
		artifact = strings.TrimSpace(artifact)
		if artifact == "" {
			continue
		}
		report.ExpectedChecked = append(report.ExpectedChecked, artifact)
		target := filepath.Join(productPath, filepath.Clean(artifact))
		if !inside(productPath, target) {
			report.Errors = append(report.Errors, "expected artifact escapes product path: "+artifact)
			continue
		}
		if _, err := os.Stat(target); err != nil {
			report.Errors = append(report.Errors, "missing expected artifact: "+artifact)
		}
	}
	if len(manifest.TestCommands) == 0 {
		report.Errors = append(report.Errors, "test_commands must not be empty")
	}
	if !hasMemoryCitation(manifest.MemoryCitations) {
		report.Errors = append(report.Errors, "memory_citations must include at least one cache_item: or wiki_item: source")
	}
	if manifest.DeliveryPath == "" {
		report.Warnings = append(report.Warnings, "delivery_path is empty")
	}
	return finish(report)
}

func finish(report ValidationReport) ValidationReport {
	report.Passed = len(report.Errors) == 0
	return report
}

func hasMemoryCitation(citations []string) bool {
	for _, citation := range citations {
		citation = strings.TrimSpace(citation)
		if strings.HasPrefix(citation, "cache_item:") || strings.HasPrefix(citation, "wiki_item:") {
			return true
		}
	}
	return false
}

func inside(root, path string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "..")
}
