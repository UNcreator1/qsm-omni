package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type HarnessMode string

const (
	HarnessSimulated HarnessMode = "simulated"
	HarnessOpenCode  HarnessMode = "opencode"
	HarnessLangChain HarnessMode = "langchain"
)

type Config struct {
	HarnessMode     HarnessMode `json:"harness_mode"`
	NineRouterURL   string      `json:"nine_router_url"`
	NineRouterKey   string      `json:"-"`
	NineRouterApp   string      `json:"nine_router_app"`
	OpenCodePath    string      `json:"opencode_path"`
	OpenCodeConfig  string      `json:"opencode_config"`
	PythonPath      string      `json:"python_path"`
	DeepAgentsRoot  string      `json:"deepagents_root"`
	OpenHarnessRoot string      `json:"openharness_root"`
	LangChainRunner string      `json:"langchain_runner"`
	LakePath        string      `json:"lake_path"`
	WikiPath        string      `json:"wiki_path"`
	RoomsPath       string      `json:"rooms_path"`
}

type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func Load(root string, mode HarnessMode) Config {
	if mode == "" {
		mode = HarnessSimulated
	}
	root = absPath(root)
	nemoRoot := getenv("QSM_NEMOCLAW_ROOT", "/Users/nexus/Downloads/NemoClaw")
	openCodeConfig := getenv("QSM_OPENCODE_CONFIG", filepath.Join(nemoRoot, "cli_proxy_api", "opencode.json"))
	routerURL, routerKey := routerFromOpenCodeConfig(openCodeConfig)
	if routerURL == "" {
		routerURL = "http://localhost:20128/v1"
	}
	return Config{
		HarnessMode:     mode,
		NineRouterURL:   getenv("QSM_9ROUTER_URL", routerURL),
		NineRouterKey:   getenv("QSM_9ROUTER_API_KEY", routerKey),
		NineRouterApp:   getenv("QSM_9ROUTER_APP", filepath.Join(nemoRoot, "projects", "sandbox_9router")),
		OpenCodePath:    getenv("QSM_OPENCODE_PATH", filepath.Join(nemoRoot, "opencode")),
		OpenCodeConfig:  openCodeConfig,
		PythonPath:      getenv("QSM_PYTHON", defaultPython(root)),
		DeepAgentsRoot:  getenv("QSM_DEEPAGENTS_ROOT", filepath.Join(nemoRoot, "deepagents_study")),
		OpenHarnessRoot: getenv("QSM_OPENHARNESS_PATH", filepath.Join(nemoRoot, "OpenHarness")),
		LangChainRunner: getenv("QSM_LANGCHAIN_RUNNER", filepath.Join(root, "harness", "langchain_runner.py")),
		LakePath:        filepath.Join(root, ".lake"),
		WikiPath:        filepath.Join(root, "internal", "wiki", "wiki.md"),
		RoomsPath:       filepath.Join(root, ".rooms"),
	}
}

func (c Config) ValidateForRealHarness() error {
	if c.HarnessMode == HarnessSimulated {
		return nil
	}
	if c.NineRouterURL == "" {
		return errors.New("QSM_9ROUTER_URL is required")
	}
	if c.NineRouterKey == "" {
		return errors.New("QSM_9ROUTER_API_KEY is required for real agent API calls")
	}
	if _, err := os.Stat(c.WikiPath); err != nil {
		return fmt.Errorf("wiki memory not available at %s: %w", c.WikiPath, err)
	}
	switch c.HarnessMode {
	case HarnessOpenCode:
		if _, err := os.Stat(c.OpenCodePath); err != nil {
			return fmt.Errorf("OpenCode CLI not available at %s: %w", c.OpenCodePath, err)
		}
		if _, err := os.Stat(c.OpenCodeConfig); err != nil {
			return fmt.Errorf("OpenCode config not available at %s: %w", c.OpenCodeConfig, err)
		}
	case HarnessLangChain:
		if _, err := os.Stat(c.LangChainRunner); err != nil {
			return fmt.Errorf("LangChain runner not available at %s: %w", c.LangChainRunner, err)
		}
		if check := c.pythonImportCheck("deepagents"); !check.OK {
			return errors.New("deepagents Python package is not importable")
		}
		if check := c.pythonImportCheck("langchain"); !check.OK {
			return errors.New("langchain Python package is not importable")
		}
	default:
		return fmt.Errorf("unknown harness mode %q", c.HarnessMode)
	}
	return nil
}

func (c Config) Doctor() []Check {
	checks := []Check{
		{Name: "harness_mode", OK: c.HarnessMode != "", Detail: string(c.HarnessMode)},
		{Name: "9router_url", OK: c.NineRouterURL != "", Detail: c.NineRouterURL},
		{Name: "9router_api_key", OK: c.NineRouterKey != "", Detail: redacted(c.NineRouterKey)},
		fileCheck("9router_app", c.NineRouterApp),
		fileCheck("opencode_cli", c.OpenCodePath),
		fileCheck("opencode_config", c.OpenCodeConfig),
		fileCheck("python", c.PythonPath),
		fileCheck("deepagents_root", c.DeepAgentsRoot),
		fileCheck("openharness_root", c.OpenHarnessRoot),
		fileCheck("langchain_runner", c.LangChainRunner),
		fileCheck("lake", c.LakePath),
		fileCheck("wiki_memory", c.WikiPath),
		fileCheck("rooms", c.RoomsPath),
	}
	checks = append(checks, c.routerLiveCheck())
	if c.HarnessMode == HarnessLangChain {
		checks = append(checks, c.pythonImportCheck("deepagents"), c.pythonImportCheck("langchain"))
	}
	return checks
}

func routerFromOpenCodeConfig(path string) (string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	var raw struct {
		Provider map[string]struct {
			Options struct {
				BaseURL string `json:"baseURL"`
				APIKey  string `json:"apiKey"`
			} `json:"options"`
		} `json:"provider"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", ""
	}
	for _, id := range []string{"oc", "kr", "or"} {
		if p, ok := raw.Provider[id]; ok && p.Options.BaseURL != "" {
			return p.Options.BaseURL, p.Options.APIKey
		}
	}
	return "", ""
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func absPath(path string) string {
	if path == "" {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func defaultPython(root string) string {
	venvPython := filepath.Join(root, ".venv", "bin", "python")
	if _, err := os.Stat(venvPython); err == nil {
		return venvPython
	}
	return "python3"
}

func fileCheck(name, path string) Check {
	if path != "" && !strings.ContainsAny(path, `/\`) {
		resolved, err := exec.LookPath(path)
		if err != nil {
			return Check{Name: name, OK: false, Detail: path + " missing from PATH"}
		}
		return Check{Name: name, OK: true, Detail: resolved}
	}
	_, err := os.Stat(path)
	if err != nil {
		return Check{Name: name, OK: false, Detail: path + " missing"}
	}
	return Check{Name: name, OK: true, Detail: path}
}

func (c Config) pythonImportCheck(module string) Check {
	cmd := exec.Command(c.PythonPath, "-c", "import "+module+"; print("+module+".__file__ if hasattr("+module+", '__file__') else 'ok')")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Check{Name: "python_import_" + module, OK: false, Detail: strings.TrimSpace(string(out))}
	}
	return Check{Name: "python_import_" + module, OK: true, Detail: strings.TrimSpace(string(out))}
}

func redacted(value string) string {
	if value == "" {
		return "not set"
	}
	if len(value) <= 6 {
		return "set"
	}
	return value[:3] + "..." + value[len(value)-3:]
}

func (c Config) routerLiveCheck() Check {
	if c.NineRouterURL == "" {
		return Check{Name: "9router_live", OK: false, Detail: "no URL"}
	}
	url := strings.TrimRight(c.NineRouterURL, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Check{Name: "9router_live", OK: false, Detail: err.Error()}
	}
	if c.NineRouterKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.NineRouterKey)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Check{Name: "9router_live", OK: false, Detail: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return Check{Name: "9router_live", OK: true, Detail: resp.Status}
	}
	return Check{Name: "9router_live", OK: false, Detail: resp.Status}
}
