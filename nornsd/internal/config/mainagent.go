package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// CopyMainAgent preserves the file bytes. Norns performs its own YAML and
// runtime placeholder updates after copying; ordinary mode does no rewriting.
func CopyMainAgent(source, agentsDir string) (filename, name string, err error) {
	info, err := os.Lstat(source)
	if err != nil {
		return "", "", fmt.Errorf("read AGENT_MAINAGENT_FILE: %w", err)
	}
	if !info.Mode().IsRegular() || filepath.Ext(source) != ".md" {
		return "", "", fmt.Errorf("AGENT_MAINAGENT_FILE must be a regular .md file, not a symlink")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return "", "", err
	}
	name, err = MainAgentName(data, source)
	if err != nil {
		return "", "", err
	}
	filename = filepath.Base(source)
	target := filepath.Join(agentsDir, filename)
	if existing, readErr := os.ReadFile(target); readErr == nil {
		if !bytes.Equal(existing, data) {
			return "", "", fmt.Errorf("main Agent filename conflicts with existing Agent %s", filename)
		}
		return filename, name, nil
	} else if !os.IsNotExist(readErr) {
		return "", "", readErr
	}
	if err = os.MkdirAll(agentsDir, 0700); err != nil {
		return "", "", err
	}
	err = os.WriteFile(target, data, 0600)
	return filename, name, err
}

func MainAgentName(data []byte, source string) (string, error) {
	text := strings.TrimPrefix(string(data), "\ufeff")
	first := strings.IndexByte(text, '\n')
	if first < 0 || strings.TrimSuffix(text[:first], "\r") != "---" {
		return "", fmt.Errorf("invalid main Agent %s: missing YAML frontmatter", source)
	}
	start := first + 1
	for offset := start; offset < len(text); {
		end := strings.IndexByte(text[offset:], '\n')
		next := len(text)
		if end < 0 {
			end = len(text)
		} else {
			end += offset
			next = end + 1
		}
		if strings.TrimSuffix(text[offset:end], "\r") == "---" {
			var front struct {
				Name string `yaml:"name"`
			}
			if err := yaml.Unmarshal([]byte(text[start:offset]), &front); err != nil {
				return "", fmt.Errorf("invalid main Agent %s: %w", source, err)
			}
			if front.Name == "" || front.Name != strings.TrimSpace(front.Name) || strings.ContainsAny(front.Name, "\t\r\n ,():") {
				return "", fmt.Errorf("invalid main Agent name %q in %s", front.Name, source)
			}
			return front.Name, nil
		}
		offset = next
	}
	return "", fmt.Errorf("invalid main Agent %s: unterminated YAML frontmatter", source)
}
