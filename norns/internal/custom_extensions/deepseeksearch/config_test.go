package deepseeksearch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigurationYAMLAndJSON(t *testing.T) {
	for _, test := range []struct {
		source  string
		base    string
		timeout int
	}{
		{"api_key: test-secret\n", "https://api.deepseek.com", 30},
		{`{"api_key":"test-secret","base_url":"https://proxy.test/v1/","timeout_seconds":90}`, "https://proxy.test", 90},
	} {
		filename := filepath.Join(t.TempDir(), "config")
		if err := os.WriteFile(filename, []byte(test.source), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(ConfigEnv, filename)
		config, err := LoadConfigFromEnv()
		if err != nil || config.APIKey != "test-secret" || config.BaseURL != test.base || config.TimeoutSeconds != test.timeout {
			t.Fatalf("config=%+v err=%v", config, err)
		}
	}
}

func TestConfigurationRequiresFileAndRejectsInvalidValues(t *testing.T) {
	t.Setenv(ConfigEnv, "")
	_, err := LoadConfigFromEnv()
	assertErrorCode(t, err, "config_error")
	t.Setenv(ConfigEnv, filepath.Join(t.TempDir(), "missing"))
	_, err = LoadConfigFromEnv()
	assertErrorCode(t, err, "config_error")
	for _, source := range []string{
		"", "null", "[]", "{}", "api_key: ''", "api_key: [test-secret]",
		"api_key: test-secret\nunknown: true", "api_key: test-secret\napi_key: other",
		"api_key: test-secret\n---\napi_key: other", "api_key: test-secret\n---\n",
		"api_key: test-secret\nbase_url: ftp://example.test",
		"api_key: test-secret\nbase_url: https://user:pass@example.test",
		"api_key: test-secret\nbase_url: https://example.test?a=b",
		"api_key: test-secret\nbase_url: https://example.test#section",
		"api_key: test-secret\ntimeout_seconds: -1",
		"api_key: test-secret\ntimeout_seconds: 1.5",
		"api_key: test-secret\ntimeout_seconds: 9223372036854775807",
		"api_key: \"test-secret\\nheader\"",
	} {
		t.Run(source, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "config")
			if err := os.WriteFile(filename, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv(ConfigEnv, filename)
			_, err := LoadConfigFromEnv()
			assertErrorCode(t, err, "config_error")
			if strings.Contains(err.Error(), "test-secret") {
				t.Fatal("configuration errors must not expose the API key")
			}
		})
	}
}
