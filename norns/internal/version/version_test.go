package version

import (
	"testing"

	versions "github.com/Ghr07h/norns"
)

func TestVersionsComeFromManifest(t *testing.T) {
	want := versions.Read().Norns
	if ProductVersion != want.Version || ProtocolVersion != want.SupportProtocol || SchemaVersion != want.DatabaseSchema {
		t.Fatalf("Norns versions do not match versions.json: %s, %d, %d", ProductVersion, ProtocolVersion, SchemaVersion)
	}
}
