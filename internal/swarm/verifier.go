package swarm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var htmlRefPattern = regexp.MustCompile(`(?i)(src|href)=["']([^"']+)["']`)

func VerifyProduct(path string) ProductVerification {
	v := ProductVerification{Type: "generic"}
	if path == "" {
		v.Errors = append(v.Errors, "product path is empty")
		return v
	}
	info, err := os.Stat(path)
	if err != nil {
		v.Errors = append(v.Errors, fmt.Sprintf("product path missing: %v", err))
		return v
	}
	if !info.IsDir() {
		v.Errors = append(v.Errors, "product path is not a directory")
		return v
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		v.Errors = append(v.Errors, err.Error())
		return v
	}
	if len(entries) == 0 {
		v.Errors = append(v.Errors, "product directory is empty")
		return v
	}
	v.Checks = append(v.Checks, "product directory is non-empty")
	index := filepath.Join(path, "index.html")
	if _, err := os.Stat(index); err == nil {
		v.Type = "static-web"
		verifyStaticWeb(path, index, &v)
	}
	if len(v.Errors) == 0 {
		v.Passed = true
	}
	return v
}

func verifyStaticWeb(productDir, indexPath string, v *ProductVerification) {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		v.Errors = append(v.Errors, fmt.Sprintf("cannot read index.html: %v", err))
		return
	}
	html := string(data)
	lower := strings.ToLower(html)
	if !strings.Contains(lower, "<html") {
		v.Errors = append(v.Errors, "index.html is missing <html")
	} else {
		v.Checks = append(v.Checks, "index.html has an html document")
	}
	if !strings.Contains(lower, "<script") && !strings.Contains(lower, "<canvas") && !strings.Contains(lower, ".js") {
		v.Errors = append(v.Errors, "static web product has no script, canvas, or local JavaScript reference")
	} else {
		v.Checks = append(v.Checks, "static web product has interactive/script surface")
	}
	refs := referencedLocalAssets(html)
	for _, ref := range refs {
		target := filepath.Join(productDir, filepath.Clean(ref))
		if _, err := os.Stat(target); err != nil {
			v.Errors = append(v.Errors, fmt.Sprintf("missing referenced asset: %s", ref))
			continue
		}
		v.Checks = append(v.Checks, "asset exists: "+ref)
	}
	verifyJavaScript(productDir, refs, v)
}

func referencedLocalAssets(html string) []string {
	seen := map[string]bool{}
	var refs []string
	for _, match := range htmlRefPattern.FindAllStringSubmatch(html, -1) {
		if len(match) < 3 {
			continue
		}
		value := strings.TrimSpace(match[2])
		if value == "" || strings.HasPrefix(value, "#") || strings.HasPrefix(value, "http:") || strings.HasPrefix(value, "https:") || strings.HasPrefix(value, "data:") || strings.HasPrefix(value, "mailto:") {
			continue
		}
		value = strings.TrimPrefix(value, "/")
		if !seen[value] {
			seen[value] = true
			refs = append(refs, value)
		}
	}
	return refs
}

func verifyJavaScript(productDir string, refs []string, v *ProductVerification) {
	jsFiles := map[string]bool{}
	for _, ref := range refs {
		if strings.EqualFold(filepath.Ext(ref), ".js") {
			jsFiles[filepath.Join(productDir, filepath.Clean(ref))] = true
		}
	}
	_ = filepath.WalkDir(productDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".js") {
			return nil
		}
		jsFiles[path] = true
		return nil
	})
	if len(jsFiles) == 0 {
		return
	}
	if _, err := exec.LookPath("node"); err != nil {
		v.Warnings = append(v.Warnings, "node is not available; skipped JavaScript syntax checks")
		return
	}
	for path := range jsFiles {
		cmd := exec.Command("node", "--check", path)
		out, err := cmd.CombinedOutput()
		rel, _ := filepath.Rel(productDir, path)
		if err != nil {
			v.Errors = append(v.Errors, fmt.Sprintf("JavaScript syntax failed for %s: %s", rel, strings.TrimSpace(string(out))))
			continue
		}
		v.Checks = append(v.Checks, "JavaScript syntax ok: "+rel)
	}
}
