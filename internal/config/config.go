// Package config loads an optional YAML file used only to seed a new database.
package config

import (
	"errors"
	"os"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"gopkg.in/yaml.v3"
)

// File is the on-disk seed document.
type File struct {
	Addr     string         `yaml:"addr"`
	DataDir  string         `yaml:"data_dir"`
	Profile  model.Profile  `yaml:"profile"`
	Settings model.Settings `yaml:"settings"`
	Sources  []model.Source `yaml:"sources"`
}

// Load reads path. A missing file is not an error.
func Load(path string) (File, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, false, nil
		}
		return File{}, false, err
	}
	var f File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return File{}, false, err
	}
	return f, true, nil
}
