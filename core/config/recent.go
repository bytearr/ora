package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

const recentMax = 20

func LoadRecent() ([]string, error) {
	path, err := RecentFile()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, nil
	}
	return ids, nil
}

// PushRecent moves id to the front and keeps the last recentMax entries.
func PushRecent(ids []string, id string) []string {
	out := []string{id}
	for _, x := range ids {
		if x != id && len(out) < recentMax {
			out = append(out, x)
		}
	}
	return out
}

func SaveRecent(ids []string) error {
	path, err := RecentFile()
	if err != nil {
		return err
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, data)
}
