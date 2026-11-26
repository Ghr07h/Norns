package runtimeartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	ccagent "github.com/Ghr07h/norns/nornsd/internal/agent/cc"
	"github.com/Ghr07h/norns/nornsd/internal/config"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	"github.com/google/uuid"
)

type Input struct {
	Subject          domain.Subject
	Runtime          domain.Runtime
	Launch           ccagent.Launch
	Managed          bool
	Config           *config.Config
	ConfigHash       string
	PluginHash       string
	CustomPluginHash string
	Slot             *int
	CreatedAt        time.Time
}

func Publish(subjectPath string, input Input) (map[string]any, error) {
	argv, err := ccagent.BuildArgs(input.Launch, input.Managed)
	if err != nil {
		return nil, err
	}
	now := input.CreatedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	profile := input.Runtime.Profile
	entries := []any{}
	for _, path := range input.Launch.PluginPaths {
		entry := map[string]any{"path": path}
		if manifest, err := os.ReadFile(filepath.Join(path, "plugin.manifest.json")); err == nil {
			hash := sha256.Sum256(manifest)
			entry["hash"] = "sha256:" + hex.EncodeToString(hash[:])
		}
		entries = append(entries, entry)
	}
	configuration := map[string]any{"slot": input.Slot, "hash": input.ConfigHash}
	if input.Config != nil {
		configuration["source"] = input.Config.Source
	}
	snapshot := map[string]any{
		"runtime": map[string]any{"id": input.Runtime.ID, "subject_id": input.Subject.ID, "created_at": now, "managed": input.Managed},
		"session": map[string]any{"id": input.Launch.SessionID, "mode": input.Runtime.SessionMode, "resume_session_id": input.Launch.ResumeSessionID},
		"agent":   map[string]any{"type": profile.Agent, "version": profile.AgentVersion, "capabilities": profile.Capabilities, "argv": argv},
		"config":  configuration,
		"plugins": entries,
	}
	if err = writeJSONAtomic(filepath.Join(root.RuntimeDataPath(subjectPath, input.Runtime.ID), "launch.json"), snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}
func writeJSONAtomic(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temp := path + "." + uuid.NewString() + ".tmp"
	defer os.Remove(temp)
	if err = os.WriteFile(temp, b, 0600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
