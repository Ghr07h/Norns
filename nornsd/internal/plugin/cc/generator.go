package cc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Ghr07h/norns/nornsd/internal/assets"
	"github.com/Ghr07h/norns/nornsd/internal/config"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	"github.com/Ghr07h/norns/nornsd/internal/version"
)

type Manifest struct {
	ProtocolVersion  int               `json:"protocol_version"`
	GeneratorVersion int               `json:"generator_version"`
	RuntimeID        string            `json:"runtime_id"`
	Files            map[string]string `json:"files"`
}

type agentFrontMatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tools       toolList `yaml:"tools"`
	Skills      []string `yaml:"skills"`
	Model       string   `yaml:"model"`
	Effort      string   `yaml:"effort"`
	Color       string   `yaml:"color"`
}

type toolList []string

func (t *toolList) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("tools must be a comma-separated scalar")
	}
	parsed, err := parseToolList(node.Value)
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

func (t toolList) MarshalYAML() (any, error) {
	return strings.Join(t, ", "), nil
}

func Publish(subjectPath, runtimeID string, c *config.Config) (string, string, error) {
	embeddedVersion, err := assets.ProtocolVersion()
	if err != nil {
		return "", "", err
	}
	if embeddedVersion != version.NornsProtocol {
		return "", "", fmt.Errorf("embedded Norns protocol %d does not match supported protocol %d", embeddedVersion, version.NornsProtocol)
	}
	stage := filepath.Join(subjectPath, ".nornsd", "staging", "plugin-"+runtimeID)
	_ = os.RemoveAll(stage)
	defer os.RemoveAll(stage)
	if err := copyEmbedded(stage); err != nil {
		return "", "", err
	}
	maxConcurrent := 3
	explorationTimeoutSeconds := 1200
	allowHuman := false
	var domain *config.Domain
	if c != nil {
		maxConcurrent = c.NornsMaxConcurrentExplorations
		if maxConcurrent == 0 {
			maxConcurrent = 3
		}
		explorationTimeoutSeconds = c.NornsExplorationTimeoutSeconds
		if explorationTimeoutSeconds == 0 {
			explorationTimeoutSeconds = 1200
		}
		allowHuman = c.NornsAllowHumanIntervention
		domain = c.Domain
	}
	reserved := map[string]bool{"coordinator.md": true, "decider.md": true, "explorer.md": true, "norns-loop": true}
	if domain != nil {
		deciderName := "decider-for-" + domain.Name
		explorerName := "explorer-for-" + domain.Name
		if err = generateDomainAgent(stage, "decider.md", deciderName+".md", deciderName, domain.DeciderDescription, domain.DeciderExtraInfo); err != nil {
			return "", "", err
		}
		if err = generateDomainAgent(stage, "explorer.md", explorerName+".md", explorerName, domain.ExplorerDescription, domain.ExplorerExtraInfo); err != nil {
			return "", "", err
		}
		reserved[deciderName+".md"] = true
		reserved[explorerName+".md"] = true
	}
	if c != nil {
		for _, item := range []struct {
			source, target string
			skill          bool
		}{{c.NornsCustomAgentsDir, "agents", false}, {c.NornsCustomSkillsDir, "skills", true}} {
			if item.source == "" {
				continue
			}
			entries, err := os.ReadDir(item.source)
			if err != nil {
				return "", "", err
			}
			for _, entry := range entries {
				if reserved[entry.Name()] {
					return "", "", fmt.Errorf("custom Norns resource conflicts with reserved name %s", entry.Name())
				}
			}
			if err = root.CopyTreeNoLinks(item.source, filepath.Join(stage, item.target)); err != nil {
				return "", "", err
			}
		}
	}
	agentIDs, err := coordinatorAgentIDs(stage, c)
	if err != nil {
		return "", "", err
	}
	humanInstruction := "执行过程中不要等待人类实时观察或回答问题；可以接收人类主动提供的新信息，但必须按自治流程继续推进。"
	if allowHuman {
		humanInstruction = "允许在遇到会实质改变结论且无法通过 Exploration 解决的阻塞问题时向人类提问并等待反馈。"
	}
	runtimeConfig := fmt.Sprintf("- 新建 Task 时执行 `norns task open ... --concurrency %d`，并以 %d 作为额定 Exploration 并发数。\n- Exploration 超时问询周期为 %d 秒；按“任务超时与取消”中的规则处理 `duration_seconds` 达到阈值及其整数倍的 running Exploration。\n- %s", maxConcurrent, maxConcurrent, explorationTimeoutSeconds, humanInstruction)
	if err = rewriteCoordinator(filepath.Join(stage, "agents", "coordinator.md"), agentIDs, runtimeConfig); err != nil {
		return "", "", err
	}
	manifest := Manifest{ProtocolVersion: version.NornsProtocol, GeneratorVersion: 1, RuntimeID: runtimeID, Files: map[string]string{}}
	var paths []string
	_ = filepath.WalkDir(stage, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(stage, path)
			paths = append(paths, rel)
		}
		return nil
	})
	sort.Strings(paths)
	for _, rel := range paths {
		b, err := os.ReadFile(filepath.Join(stage, rel))
		if err != nil {
			return "", "", err
		}
		sum := sha256.Sum256(b)
		manifest.Files[filepath.ToSlash(rel)] = "sha256:" + hex.EncodeToString(sum[:])
	}
	b, _ := json.MarshalIndent(manifest, "", "  ")
	b = append(b, '\n')
	if err := os.WriteFile(filepath.Join(stage, "plugin.manifest.json"), b, 0600); err != nil {
		return "", "", err
	}
	artifact := filepath.Join(subjectPath, ".nornsd", "runtimes", runtimeID, "artifacts", "plugin")
	if err := os.MkdirAll(filepath.Dir(artifact), 0700); err != nil {
		return "", "", err
	}
	_ = os.RemoveAll(artifact)
	if err := os.Rename(stage, artifact); err != nil {
		return "", "", err
	}
	current := filepath.Join(subjectPath, ".nornsd", "plugin")
	published := current + ".stage"
	_ = os.RemoveAll(published)
	if err := root.CopyTreeNoLinks(artifact, published); err != nil {
		return "", "", err
	}
	old := current + ".old"
	_ = os.RemoveAll(old)
	_ = os.Rename(current, old)
	if err := os.Rename(published, current); err != nil {
		_ = os.Rename(old, current)
		return "", "", err
	}
	_ = os.RemoveAll(old)
	sum := sha256.Sum256(b)
	return current, "sha256:" + hex.EncodeToString(sum[:]), nil
}

func generateDomainAgent(stage, sourceName, targetName, agentName, description, extraInfo string) error {
	source, err := os.ReadFile(filepath.Join(stage, "agents", sourceName))
	if err != nil {
		return err
	}
	front, body, err := parseAgentMarkdown(source, sourceName)
	if err != nil {
		return err
	}
	front.Name = agentName
	front.Description = description
	rendered, err := renderAgentMarkdown(front, body)
	if err != nil {
		return err
	}
	if extraInfo != "" {
		if !strings.HasSuffix(rendered, "\n") {
			rendered += "\n"
		}
		rendered += "\n" + extraInfo
		if !strings.HasSuffix(rendered, "\n") {
			rendered += "\n"
		}
	}
	return os.WriteFile(filepath.Join(stage, "agents", targetName), []byte(rendered), 0600)
}

func coordinatorAgentIDs(stage string, c *config.Config) ([]string, error) {
	seen := map[string]string{}
	var ids []string
	if err := appendAgentIDs(&ids, seen, filepath.Join(stage, "agents"), "norns", map[string]bool{"coordinator.md": true}); err != nil {
		return nil, err
	}
	if c == nil {
		return ids, nil
	}
	if err := appendAgentIDs(&ids, seen, c.CustomAgentsDir, "nornsd-custom", nil); err != nil {
		return nil, err
	}
	for _, pluginDir := range c.CustomPluginDirs {
		agentsDir := filepath.Join(pluginDir, "agents")
		if _, err := os.Stat(agentsDir); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("inspect custom Plugin agents %s: %w", pluginDir, err)
		}
		namespace, err := readPluginName(pluginDir)
		if err != nil {
			return nil, err
		}
		if err = appendAgentIDs(&ids, seen, agentsDir, namespace, nil); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func appendAgentIDs(ids *[]string, seen map[string]string, dir, namespace string, excludedFiles map[string]bool) error {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Agent directory %s: %w", dir, err)
	}
	if err = validateAgentIdentifierPart(namespace, "Plugin name"); err != nil {
		return err
	}
	var discovered []string
	for _, entry := range entries {
		if entry.IsDir() || excludedFiles[entry.Name()] || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read Agent %s: %w", path, readErr)
		}
		name, parseErr := parseAgentName(data, path)
		if parseErr != nil {
			return parseErr
		}
		if err = validateAgentIdentifierPart(name, "Agent name"); err != nil {
			return fmt.Errorf("invalid Agent %s: %w", path, err)
		}
		id := namespace + ":" + name
		if id == "norns:coordinator" {
			return fmt.Errorf("custom Agent %s conflicts with root Agent %s", path, id)
		}
		if previous, exists := seen[id]; exists {
			return fmt.Errorf("Agent %s is declared by both %s and %s", id, previous, path)
		}
		seen[id] = path
		discovered = append(discovered, id)
	}
	sort.Strings(discovered)
	*ids = append(*ids, discovered...)
	return nil
}

func readPluginName(pluginDir string) (string, error) {
	path := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read custom Plugin metadata %s: %w", path, err)
	}
	var metadata struct {
		Name string `json:"name"`
	}
	if err = json.Unmarshal(data, &metadata); err != nil {
		return "", fmt.Errorf("invalid custom Plugin metadata %s: %w", path, err)
	}
	if err = validateAgentIdentifierPart(metadata.Name, "Plugin name"); err != nil {
		return "", fmt.Errorf("invalid custom Plugin metadata %s: %w", path, err)
	}
	return metadata.Name, nil
}

func validateAgentIdentifierPart(value, field string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\t\r\n ,():") {
		return fmt.Errorf("%s %q cannot be used in an Agent tool allowlist", field, value)
	}
	return nil
}

func rewriteCoordinator(path string, agentIDs []string, runtimeConfig string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	front, body, err := parseAgentMarkdown(data, path)
	if err != nil {
		return err
	}
	front.Tools, err = front.Tools.withAgentAllowlist(agentIDs)
	if err != nil {
		return fmt.Errorf("invalid Coordinator tools: %w", err)
	}
	body = strings.ReplaceAll(body, "{{NORNS_RUNTIME_CONFIGURATION}}", runtimeConfig)
	rendered, err := renderAgentMarkdown(front, body)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(rendered), 0600)
}

func (t toolList) withAgentAllowlist(agentIDs []string) (toolList, error) {
	out := append(toolList(nil), t...)
	found := false
	for i, tool := range out {
		name := tool
		if index := strings.IndexByte(name, '('); index >= 0 {
			name = strings.TrimSpace(name[:index])
		}
		if name != "Agent" {
			continue
		}
		if found {
			return nil, fmt.Errorf("multiple Agent tool declarations")
		}
		found = true
		out[i] = fmt.Sprintf("Agent(%s)", strings.Join(agentIDs, ", "))
	}
	if !found {
		return nil, fmt.Errorf("missing Agent tool declaration")
	}
	return out, nil
}

func parseToolList(value string) (toolList, error) {
	var tools toolList
	start := 0
	depth := 0
	for i, r := range value {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("invalid tools value: unmatched closing parenthesis")
			}
		case ',':
			if depth == 0 {
				tool := strings.TrimSpace(value[start:i])
				if tool == "" {
					return nil, fmt.Errorf("invalid tools value: empty tool")
				}
				tools = append(tools, tool)
				start = i + 1
			}
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("invalid tools value: unmatched opening parenthesis")
	}
	tool := strings.TrimSpace(value[start:])
	if tool == "" {
		return nil, fmt.Errorf("invalid tools value: empty tool")
	}
	return append(tools, tool), nil
}

func parseAgentMarkdown(data []byte, source string) (agentFrontMatter, string, error) {
	frontBytes, body, err := splitAgentMarkdown(data, source)
	if err != nil {
		return agentFrontMatter{}, "", err
	}
	var front agentFrontMatter
	if err = yaml.Unmarshal(frontBytes, &front); err != nil {
		return agentFrontMatter{}, "", fmt.Errorf("invalid Agent %s front matter: %w", source, err)
	}
	return front, body, nil
}

func parseAgentName(data []byte, source string) (string, error) {
	frontBytes, _, err := splitAgentMarkdown(data, source)
	if err != nil {
		return "", err
	}
	var front struct {
		Name string `yaml:"name"`
	}
	if err = yaml.Unmarshal(frontBytes, &front); err != nil {
		return "", fmt.Errorf("invalid Agent %s front matter: %w", source, err)
	}
	return front.Name, nil
}

func splitAgentMarkdown(data []byte, source string) ([]byte, string, error) {
	const prefix = "---\n"
	const separator = "\n---\n"
	text := string(data)
	if !strings.HasPrefix(text, prefix) {
		return nil, "", fmt.Errorf("invalid Agent %s: missing front matter", source)
	}
	rest := strings.TrimPrefix(text, prefix)
	index := strings.Index(rest, separator)
	if index < 0 {
		return nil, "", fmt.Errorf("invalid Agent %s: unterminated front matter", source)
	}
	return []byte(rest[:index]), rest[index+len(separator):], nil
}

func renderAgentMarkdown(front agentFrontMatter, body string) (string, error) {
	frontBytes, err := yaml.Marshal(front)
	if err != nil {
		return "", err
	}
	return "---\n" + string(frontBytes) + "---\n" + body, nil
}

func copyEmbedded(target string) error {
	const sourceRoot = "generated/plugins/cc"
	return fs.WalkDir(assets.Generated, sourceRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, sourceRoot)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return os.MkdirAll(target, 0700)
		}
		dst := filepath.Join(target, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(dst, 0700)
		}
		b, err := assets.Generated.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0600)
	})
}
