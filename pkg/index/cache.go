package index

import (
	"encoding/json"
	"os"
	"path/filepath"

	"gts-suite/pkg/model"
)

func Save(path string, idx *model.Index) error {
	if idx == nil {
		return nil
	}

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(idx)
}

func Load(path string) (*model.Index, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var idx model.Index
	if err := json.NewDecoder(file).Decode(&idx); err != nil {
		return nil, err
	}
	return &idx, nil
}
