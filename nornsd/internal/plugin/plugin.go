package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Ghr07h/norns/nornsd/internal/config"
	plugincc "github.com/Ghr07h/norns/nornsd/internal/plugin/cc"
)

type Publication struct {
	Path      string
	Hash      string
	RootAgent string
}

type RuntimePlugins struct {
	Paths     []string
	RootAgent string
}

// Prepare is shared by managed and direct runtimes. Every returned path belongs
// to this Runtime, including when all plugin files came from an older Runtime.
func Prepare(agent, subjectPath, runtimeID string, cfg *config.Config) (RuntimePlugins, error) {
	if cfg == nil {
		return RuntimePlugins{}, fmt.Errorf("Runtime configuration is required")
	}
	if err := config.ValidateResources(*cfg); err != nil {
		return RuntimePlugins{}, err
	}
	if cfg.ReuseRuntimeDir != "" {
		return reuseRuntime(agent, subjectPath, runtimeID, cfg.ReuseRuntimeDir)
	}
	if cfg.NornsEnable {
		pub, err := Publish(agent, subjectPath, runtimeID, cfg)
		if err != nil {
			return RuntimePlugins{}, err
		}
		return RuntimePlugins{Paths: []string{pub.Path}, RootAgent: pub.RootAgent}, nil
	}
	paths, err := config.PublishExternal(subjectPath, runtimeID, cfg)
	if err != nil {
		return RuntimePlugins{}, err
	}
	custom, _, err := config.PublishCustom(subjectPath, runtimeID, cfg)
	if err != nil {
		return RuntimePlugins{}, err
	}
	mainID := ""
	if custom != "" {
		paths = append(paths, custom)
		if cfg.MainAgentFile != "" {
			path := filepath.Join(custom, "agents", filepath.Base(cfg.MainAgentFile))
			data, err := os.ReadFile(path)
			if err != nil {
				return RuntimePlugins{}, err
			}
			name, err := config.MainAgentName(data, path)
			if err != nil {
				return RuntimePlugins{}, err
			}
			mainID = "nornsd-custom:" + name
		}
	}
	return RuntimePlugins{Paths: paths, RootAgent: mainID}, nil
}

func Publish(agent, subjectPath, runtimeID string, cfg *config.Config) (Publication, error) {
	switch agent {
	case "cc":
		path, hash, mainID, err := plugincc.Publish(subjectPath, runtimeID, cfg)
		return Publication{Path: path, Hash: hash, RootAgent: mainID}, err
	default:
		return Publication{}, fmt.Errorf("Norns plugin generation is not implemented for agent %q", agent)
	}
}
