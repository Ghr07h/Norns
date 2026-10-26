package assets

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:generate go run ../../cmd/genassets --protocol ../../../documents/protocol --plugins ../../../plugin --output generated

// Generated contains deterministic Plugin resources grouped by Agent type.
//
// The all: prefix is required because Claude Plugin metadata lives in the
// hidden .claude-plugin directory.
//
//go:embed all:generated
var Generated embed.FS

func ProtocolVersion() (int, error) {
	b, err := Generated.ReadFile("generated/manifest.generated.json")
	if err != nil {
		return 0, err
	}
	var manifest struct {
		ProtocolVersion int `json:"protocol_version"`
	}
	if err = json.Unmarshal(b, &manifest); err != nil {
		return 0, fmt.Errorf("invalid generated asset manifest: %w", err)
	}
	return manifest.ProtocolVersion, nil
}
