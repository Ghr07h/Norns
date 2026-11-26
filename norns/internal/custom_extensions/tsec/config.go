package tsec

import (
	"net/url"
	"os"
	"strings"
)

const BaseURLEnv = "BENCHMARK_BASE_URL"
const TokenEnv = "BENCHMARK_TOKEN"

type Config struct {
	BaseURL string
	Token   string
}

func LoadConfigFromEnv() (Config, error) {
	return normalizeConfig(Config{BaseURL: os.Getenv(BaseURLEnv), Token: os.Getenv(TokenEnv)})
}

func normalizeConfig(config Config) (Config, error) {
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	base, err := url.Parse(config.BaseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.ForceQuery || base.Fragment != "" || strings.Contains(config.BaseURL, "#") {
		return Config{}, failure("config_error", BaseURLEnv+" must be an HTTP(S) service base URL without credentials, query or fragment")
	}
	config.Token = strings.TrimSpace(config.Token)
	if config.Token == "" {
		return Config{}, failure("config_error", TokenEnv+" must be set to a non-empty token")
	}
	for _, ch := range config.Token {
		if ch < 32 || ch > 126 {
			return Config{}, failure("config_error", TokenEnv+" must contain only printable ASCII characters")
		}
	}
	return config, nil
}
