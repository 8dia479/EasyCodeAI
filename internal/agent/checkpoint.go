package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/8dia479/EasyCodeAI/internal/nim"
)

const checkpointVersion = 1

type Checkpoint struct {
	Version      int          `json:"version"`
	Task         string       `json:"task"`
	Workspace    string       `json:"workspace"`
	Phase        string       `json:"phase"`
	Round        int          `json:"round"`
	UpdatedAt    time.Time    `json:"updated_at"`
	History      []string     `json:"history"`
	Messages     []nim.Message `json:"messages,omitempty"`
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
		return fmt.Errorf("create checkpoint directory: %w", err)
	}
	cp.Version = checkpointVersion
	cp.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("encode checkpoint: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".checkpoint-*.tmp")
	if err != nil {
		return fmt.Errorf("create checkpoint temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write checkpoint: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync checkpoint: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close checkpoint: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace checkpoint: %w", err)
	}
	return nil
}

func loadCheckpoint() (Checkpoint, error) {
	var cp Checkpoint
	data, err := os.ReadFile(statePath())
	if err != nil {
		return cp, err
	}
	if err := json.Unmarshal(data, &cp); err != nil {
		return cp, fmt.Errorf("decode checkpoint: %w", err)
	}
	if cp.Version != checkpointVersion {
		return cp, fmt.Errorf("unsupported checkpoint version %d (expected %d)", cp.Version, checkpointVersion)
	}
	if cp.Task == "" || len(cp.Messages) == 0 {
		return cp, fmt.Errorf("checkpoint does not contain a resumable conversation")
	}
	return cp, nil
}
