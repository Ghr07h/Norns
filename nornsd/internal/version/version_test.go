package version

import (
	"testing"

	versions "github.com/Ghr07h/norns"
)

func TestVersionsComeFromManifest(t *testing.T) {
	want := versions.Read().Nornsd
	if ProductVersion != want.Version || NornsProtocol != want.SupportProtocol || DatabaseSchema != want.DatabaseSchema || APISchema != want.APISchema || AgentConfigVersion != want.AgentConfigSchema || CCPluginVersion != want.Plugins["cc"] {
		t.Fatal("Nornsd versions do not match versions.json")
	}
}
