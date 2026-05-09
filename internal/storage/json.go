package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type JSONStore struct {
	DataDir string
	WebDir  string
}

func New(dataDir, webDir string) JSONStore {
	return JSONStore{DataDir: dataDir, WebDir: webDir}
}

func (s JSONStore) LatestReportPath() string { return filepath.Join(s.DataDir, "latest_report.json") }
func (s JSONStore) HistoryPath() string      { return filepath.Join(s.DataDir, "probe_history.json") }
func (s JSONStore) NotifyStatePath() string  { return filepath.Join(s.DataDir, "notify_state.txt") }

func ReadJSON[T any](path string, fallback T) (T, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fallback, nil
		}
		return fallback, err
	}
	defer file.Close()
	var value T
	if err := json.NewDecoder(file).Decode(&value); err != nil {
		return fallback, err
	}
	return value, nil
}

func WriteJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(path)
		if err := os.Rename(tmpPath, path); err != nil {
			os.Remove(tmpPath)
			return err
		}
	}
	return nil
}
