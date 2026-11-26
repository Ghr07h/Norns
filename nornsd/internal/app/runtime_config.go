package app

import (
	"encoding/json"
	"github.com/Ghr07h/norns/nornsd/internal/config"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"net/http"
	"os"
	"path/filepath"
)

func validateConfigOverride(values []json.RawMessage) (json.RawMessage, error) {
	raw := json.RawMessage(`{}`)
	if len(values) > 0 && len(values[0]) > 0 {
		raw = values[0]
	}
	parsed, err := config.ParseOverride(raw)
	if err != nil {
		return nil, domain.Wrap("INVALID_SETTING", "invalid runtime config", http.StatusBadRequest, err)
	}
	if !parsed.HasOverrides() {
		return json.RawMessage(`{}`), nil
	}
	var value map[string]json.RawMessage
	_ = json.Unmarshal(raw, &value)
	return json.Marshal(value)
}
func hasNornsWorkspace(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".norns"))
	if err != nil || !info.IsDir() {
		return false
	}
	info, err = os.Stat(filepath.Join(path, ".norns", "norns.sqlite"))
	return err == nil && info.Mode().IsRegular()
}
