package versions

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
)

//go:embed versions.json
var data []byte

type Manifest struct {
	Protocol int `json:"protocol"`
	Norns    struct {
		Version         string `json:"version"`
		SupportProtocol int    `json:"support_protocol"`
		DatabaseSchema  int    `json:"database_schema"`
	} `json:"norns"`
	Nornsd struct {
		Version           string         `json:"version"`
		SupportProtocol   int            `json:"support_protocol"`
		DatabaseSchema    int            `json:"database_schema"`
		APISchema         int            `json:"api_schema"`
		AgentConfigSchema int            `json:"agent_config_schema"`
		Plugins           map[string]int `json:"plugins"`
	} `json:"nornsd"`
}

func Read() Manifest {
	manifest, err := parse(data)
	if err != nil {
		panic(fmt.Sprintf("invalid embedded versions.json: %v", err))
	}
	return manifest
}

func parse(source []byte) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(source, &manifest); err != nil {
		return manifest, err
	}
	productVersion := regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	for name, value := range map[string]string{"norns.version": manifest.Norns.Version, "nornsd.version": manifest.Nornsd.Version} {
		if !productVersion.MatchString(value) {
			return manifest, fmt.Errorf("%s must be a three-part product version", name)
		}
	}
	for name, value := range map[string]int{
		"protocol":                   manifest.Protocol,
		"norns.support_protocol":     manifest.Norns.SupportProtocol,
		"norns.database_schema":      manifest.Norns.DatabaseSchema,
		"nornsd.support_protocol":    manifest.Nornsd.SupportProtocol,
		"nornsd.database_schema":     manifest.Nornsd.DatabaseSchema,
		"nornsd.api_schema":          manifest.Nornsd.APISchema,
		"nornsd.agent_config_schema": manifest.Nornsd.AgentConfigSchema,
		"nornsd.plugins.cc":          manifest.Nornsd.Plugins["cc"],
	} {
		if value < 1 {
			return manifest, fmt.Errorf("%s must be a positive integer", name)
		}
	}
	return manifest, nil
}
