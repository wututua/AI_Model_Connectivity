package notify

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type State struct {
	Status string    `json:"status"`
	SentAt time.Time `json:"sent_at"`
}

func readState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return State{}, nil
	}
	var value State
	if strings.HasPrefix(text, "{") {
		if err := json.Unmarshal(data, &value); err != nil {
			return State{}, err
		}
		return value, nil
	}
	return State{Status: text}, nil
}

func writeState(path string, value State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
