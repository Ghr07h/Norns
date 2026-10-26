package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const memorySamplesFilename = "memory-samples.jsonl"

type memorySample struct {
	SampledAt   time.Time `json:"sampled_at"`
	MemoryBytes uint64    `json:"memory_bytes"`
}

type memorySampleWriter struct {
	file    *os.File
	encoder *json.Encoder
}

func openMemorySampleWriter(subjectPath, runtimeID string) (*memorySampleWriter, error) {
	path := filepath.Join(subjectPath, ".nornsd", "runtimes", runtimeID, memorySamplesFilename)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &memorySampleWriter{file: file, encoder: json.NewEncoder(file)}, nil
}

func (w *memorySampleWriter) Write(sample memorySample) error {
	return w.encoder.Encode(sample)
}

func (w *memorySampleWriter) Close() error {
	return w.file.Close()
}
