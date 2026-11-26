package deepseeksearch

import (
	"bytes"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const ConfigEnv = "NORNS_EXTENSION_DEEPSEEKSEARCH_CONFIG"

// Config is read from the YAML or JSON file named by ConfigEnv.
type Config struct {
	APIKey         string `yaml:"api_key" json:"api_key"`
	BaseURL        string `yaml:"base_url" json:"base_url"`
	TimeoutSeconds int    `yaml:"timeout_seconds" json:"timeout_seconds"`
}

func LoadConfigFromEnv() (Config, error) {
	filename := strings.TrimSpace(os.Getenv(ConfigEnv))
	if filename == "" {
		return Config{}, failure("config_error", ConfigEnv+" must point to a YAML or JSON configuration file")
	}
	source, err := os.ReadFile(filename)
	if err != nil {
		return Config{}, failure("config_error", "cannot read the DeepSeek Search configuration file")
	}
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(source))
	if err := decoder.Decode(&document); err != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return Config{}, failure("config_error", "configuration must be a YAML or JSON object with api_key, base_url and timeout_seconds fields")
	}
	// yaml.v3 otherwise accepts values such as 1.5 for an int by truncation.
	fields := document.Content[0].Content
	for i := 0; i < len(fields); i += 2 {
		key, value := fields[i], fields[i+1]
		if key.Tag != "!!str" || value.Kind != yaml.ScalarNode {
			return Config{}, failure("config_error", "configuration fields must be scalar values with string keys")
		}
		switch key.Value {
		case "api_key", "base_url":
			if value.Tag != "!!str" {
				return Config{}, failure("config_error", key.Value+" must be a string")
			}
		case "timeout_seconds":
			if value.Tag != "!!int" {
				return Config{}, failure("config_error", "timeout_seconds must be an integer")
			}
		default:
			return Config{}, failure("config_error", "configuration contains an unknown field")
		}
	}
	var config Config
	if err := document.Decode(&config); err != nil {
		return Config{}, failure("config_error", "configuration contains duplicate fields or invalid values")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Config{}, failure("config_error", "configuration must contain exactly one YAML or JSON document")
	}
	return normalizeConfig(config)
}

func normalizeConfig(config Config) (Config, error) {
	config.APIKey = strings.TrimSpace(config.APIKey)
	if config.APIKey == "" || strings.ContainsAny(config.APIKey, "\r\n") {
		return Config{}, failure("config_error", "api_key must be a non-empty single-line string")
	}
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if config.BaseURL == "" {
		config.BaseURL = "https://api.deepseek.com"
	}
	base, err := url.Parse(config.BaseURL)
	if err != nil || !validHTTPURL(base) || base.RawQuery != "" || base.ForceQuery || base.Fragment != "" {
		return Config{}, failure("config_error", "base_url must be an HTTP(S) URL without credentials, query or fragment")
	}
	// Accept the common OpenAI-style base URL ending in /v1 as well.
	config.BaseURL = strings.TrimSuffix(config.BaseURL, "/v1")
	if config.TimeoutSeconds == 0 {
		config.TimeoutSeconds = 30
	}
	if config.TimeoutSeconds < 0 || int64(config.TimeoutSeconds) > int64((1<<63-1)/time.Second) {
		return Config{}, failure("config_error", "timeout_seconds must be a positive integer")
	}
	return config, nil
}

func validHTTPURL(value *url.URL) bool {
	return value != nil && (value.Scheme == "http" || value.Scheme == "https") && value.Hostname() != "" && value.User == nil
}
