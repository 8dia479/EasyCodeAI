package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Checkpoint struct {
	Task      string    `json:"task"`
	Phase     string    `json:"phase"`
	UpdatedAt time.Time `json:"updated_at"`
	History   []string  `json:"history"`
}

func statePath() string {
	if dir := os.Getenv("EASYCODE_STATE_DIR"); dir != "" {
		return filepath.Join(dir, "checkpoint.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".easycode/checkpoint.json"
	}
	return filepath.Join(home, ".easycode", "checkpoint.json")
}

func saveCheckpoint(cp Checkpoint) error {
	path := statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	cp.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadCheckpoint() (Checkpoint, error) {
	var cp Checkpoint
	data, err := os.ReadFile(statePath())
	if err != nil {
		return cp, err
	}
	err = json.Unmarshal(data, &cp)
	return cp, err
}
