package plugin

import (
	"fmt"

	"github.com/Ghr07h/norns/nornsd/internal/config"
	plugincc "github.com/Ghr07h/norns/nornsd/internal/plugin/cc"
)

type Publication struct {
	Path      string
	Hash      string
	RootAgent string
}

func Publish(agent, subjectPath, runtimeID string, cfg *config.Config) (Publication, error) {
	switch agent {
	case "cc":
		path, hash, err := plugincc.Publish(subjectPath, runtimeID, cfg)
		return Publication{Path: path, Hash: hash, RootAgent: "norns:coordinator"}, err
	default:
		return Publication{}, fmt.Errorf("Norns plugin generation is not implemented for agent %q", agent)
	}
}
