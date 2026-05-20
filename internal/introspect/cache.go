package introspect

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

const cacheVersion = 1

type cachedSchema struct {
	Version int
	Schema  Schema
}

// LoadCache reads a cached schema. It returns ok=false when the file does not
// exist or holds a version we no longer understand; an error is only returned
// when the file exists but cannot be decoded. Callers should fall back to a
// fresh Introspect call on (ok=false, err=nil).
func LoadCache(path string) (Schema, bool, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from --cache, a user-chosen file
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Schema{}, false, nil
		}

		return Schema{}, false, fmt.Errorf("open cache: %w", err)
	}
	defer func() { _ = f.Close() }()

	var c cachedSchema
	if err := gob.NewDecoder(f).Decode(&c); err != nil {
		return Schema{}, false, fmt.Errorf("decode cache: %w", err)
	}
	if c.Version != cacheVersion {
		return Schema{}, false, nil
	}

	return c.Schema, true, nil
}

// SaveCache writes the schema to path via a tmp file and rename, so a crash
// mid-write does not leave a half-written cache.
func SaveCache(path string, schema Schema) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp) //nolint:gosec // path comes from --cache, a user-chosen file
	if err != nil {
		return fmt.Errorf("create cache: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()

	if err := gob.NewEncoder(f).Encode(cachedSchema{
		Version: cacheVersion,
		Schema:  schema,
	}); err != nil {
		_ = os.Remove(tmp)

		return fmt.Errorf("encode cache: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close cache: %w", err)
	}
	closed = true
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename cache: %w", err)
	}

	return nil
}
