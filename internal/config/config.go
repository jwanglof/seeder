package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	CurrentVersion  = 1
	DefaultFilename = "seeder.yaml"
)

type Config struct {
	Version  int                    `yaml:"version"`
	Seed     *uint64                `yaml:"seed,omitempty"`
	Rows     *int                   `yaml:"rows,omitempty"`
	Locale   string                 `yaml:"locale,omitempty"`
	Truncate *bool                  `yaml:"truncate,omitempty"`
	Tables   map[string]TableConfig `yaml:"tables,omitempty"`
}

type TableConfig struct {
	Rows    *int `yaml:"rows,omitempty"`
	Exclude bool `yaml:"exclude,omitempty"`
}

// Load returns an error wrapping os.ErrNotExist when the file is missing
// so callers can use errors.Is to treat absence as a non-error.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is user-supplied via --config by design
	if err != nil {
		return Config{}, fmt.Errorf("seeder.yaml: read %s: %w", path, err)
	}

	return Parse(data)
}

func Parse(data []byte) (Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var c Config
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("seeder.yaml: %w", err)
	}

	if c.Version == 0 {
		return Config{}, errors.New("seeder.yaml: missing required field `version`")
	}
	if c.Version != CurrentVersion {
		return Config{}, fmt.Errorf(
			"seeder.yaml: unsupported version %d (this binary understands version %d)",
			c.Version, CurrentVersion,
		)
	}

	if c.Rows != nil && *c.Rows < 0 {
		return Config{}, fmt.Errorf("seeder.yaml: rows must be >= 0, got %d", *c.Rows)
	}
	for name, t := range c.Tables {
		if t.Rows != nil && *t.Rows < 0 {
			return Config{}, fmt.Errorf("seeder.yaml: tables.%s.rows must be >= 0, got %d", name, *t.Rows)
		}
	}

	return c, nil
}

// AutoDetect returns found=false (without error) when DefaultFilename is
// absent in dir, so the CLI can silently fall back to flag defaults.
func AutoDetect(dir string) (cfg Config, found bool, err error) {
	cfg, err = Load(filepath.Join(dir, DefaultFilename))
	switch {
	case err == nil:
		return cfg, true, nil
	case errors.Is(err, os.ErrNotExist):
		return Config{}, false, nil
	default:
		return Config{}, false, err
	}
}
