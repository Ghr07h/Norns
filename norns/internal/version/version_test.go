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
		Norns struct {
			Version         string `json:"version"`
			SupportProtocol int    `json:"support_protocol"`
			DatabaseSchema  int    `json:"database_schema"`
		} `json:"norns"`
	}
	if err := json.Unmarshal(source, &manifest); err != nil {
		t.Fatal(err)
	}
	want := manifest.Norns
	if ProductVersion != want.Version || ProtocolVersion != want.SupportProtocol || SchemaVersion != want.DatabaseSchema {
		t.Fatalf("Norns versions do not match versions.json; run ./release.sh norns: %s, %d, %d", ProductVersion, ProtocolVersion, SchemaVersion)
	}
}
