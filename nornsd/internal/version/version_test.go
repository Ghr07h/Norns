package version

import (
	"encoding/json"
	"os"
	"testing"
)

func TestVersionsComeFromManifest(t *testing.T) {
	source, err := os.ReadFile("../../../versions.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Nornsd struct {
			Version           string         `json:"version"`
			SupportProtocol   int            `json:"support_protocol"`
			DatabaseSchema    int            `json:"database_schema"`
			APISchema         int            `json:"api_schema"`
			AgentConfigSchema int            `json:"agent_config_schema"`
			Plugins           map[string]int `json:"plugins"`
		} `json:"nornsd"`
	}
	if err := json.Unmarshal(source, &manifest); err != nil {
		t.Fatal(err)
	}
	want := manifest.Nornsd
	if ProductVersion != want.Version || NornsProtocol != want.SupportProtocol || DatabaseSchema != want.DatabaseSchema || APISchema != want.APISchema || AgentConfigVersion != want.AgentConfigSchema || CCPluginVersion != want.Plugins["cc"] {
		t.Fatal("Nornsd versions do not match versions.json; run ./release.sh nornsd")
	}
}
