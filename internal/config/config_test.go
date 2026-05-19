package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mickamy/seeder/internal/config"
)

func TestParse_Valid(t *testing.T) {
	t.Parallel()

	in := []byte(`version: 1
seed: 42
rows: 100
locale: ja
truncate: true
tables:
  users:
    rows: 10000
  audit_log:
    exclude: true
`)
	got, err := config.Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if got.Seed == nil || *got.Seed != 42 {
		t.Errorf("Seed = %v, want 42", got.Seed)
	}
	if got.Rows == nil || *got.Rows != 100 {
		t.Errorf("Rows = %v, want 100", got.Rows)
	}
	if got.Locale != "ja" {
		t.Errorf("Locale = %q, want ja", got.Locale)
	}
	if got.Truncate == nil || !*got.Truncate {
		t.Errorf("Truncate = %v, want true", got.Truncate)
	}

	users, ok := got.Tables["users"]
	if !ok {
		t.Fatal("tables.users missing")
	}
	if users.Rows == nil || *users.Rows != 10000 {
		t.Errorf("tables.users.rows = %v, want 10000", users.Rows)
	}
	audit, ok := got.Tables["audit_log"]
	if !ok {
		t.Fatal("tables.audit_log missing")
	}
	if !audit.Exclude {
		t.Error("tables.audit_log.exclude = false, want true")
	}
}

func TestParse_MinimalVersionOnly(t *testing.T) {
	t.Parallel()

	got, err := config.Parse([]byte("version: 1\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if got.Seed != nil || got.Rows != nil || got.Truncate != nil {
		t.Errorf("unset pointer fields should be nil, got Seed=%v Rows=%v Truncate=%v",
			got.Seed, got.Rows, got.Truncate)
	}
}

func TestParse_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "missing version",
			in:   "rows: 10\n",
			want: "missing required field `version`",
		},
		{
			name: "unsupported version",
			in:   "version: 2\n",
			want: "unsupported version 2",
		},
		{
			name: "unknown field",
			in:   "version: 1\nrowz: 10\n",
			want: "field rowz not found",
		},
		{
			name: "negative rows",
			in:   "version: 1\nrows: -1\n",
			want: "rows must be >= 0",
		},
		{
			name: "negative table rows",
			in:   "version: 1\ntables:\n  users:\n    rows: -5\n",
			want: "tables.users.rows must be >= 0",
		},
		{
			name: "column both generator and value",
			in: "version: 1\ntables:\n  users:\n    columns:\n      email:\n" +
				"        generator: Email\n        value: foo\n",
			want: "cannot set both `generator` and `value`",
		},
		{
			name: "column neither generator nor value",
			in:   "version: 1\ntables:\n  users:\n    columns:\n      email: {}\n",
			want: "one of `generator` or `value` must be set",
		},
		{
			name: "column value is a map",
			in: "version: 1\ntables:\n  users:\n    columns:\n      meta:\n" +
				"        value:\n          key: foo\n",
			want: "value must be a scalar",
		},
		{
			name: "column value is a list",
			in: "version: 1\ntables:\n  users:\n    columns:\n      tags:\n" +
				"        value:\n          - a\n          - b\n",
			want: "value must be a scalar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Parse([]byte(tt.in))
			if err == nil {
				t.Fatal("Parse: nil error, want failure")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Parse error = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func TestParse_ColumnOverrides(t *testing.T) {
	t.Parallel()

	in := []byte(`version: 1
tables:
  users:
    columns:
      email:
        generator: Email
      country:
        value: JP
`)
	got, err := config.Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	users, ok := got.Tables["users"]
	if !ok {
		t.Fatal("tables.users missing")
	}
	email, ok := users.Columns["email"]
	if !ok || email.Generator != "Email" {
		t.Errorf("email override = %+v; want Generator=Email", email)
	}
	country, ok := users.Columns["country"]
	if !ok || country.Value != "JP" {
		t.Errorf("country override = %+v; want Value=JP", country)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := config.Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load: err = %v, want errors.Is os.ErrNotExist", err)
	}
}

func TestAutoDetect_Missing(t *testing.T) {
	t.Parallel()

	_, found, err := config.AutoDetect(t.TempDir())
	if err != nil {
		t.Fatalf("AutoDetect: %v", err)
	}
	if found {
		t.Error("AutoDetect found = true, want false")
	}
}

func TestAutoDetect_Found(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, config.DefaultFilename)
	if err := os.WriteFile(path, []byte("version: 1\nrows: 50\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, found, err := config.AutoDetect(dir)
	if err != nil {
		t.Fatalf("AutoDetect: %v", err)
	}
	if !found {
		t.Fatal("AutoDetect found = false, want true")
	}
	if cfg.Rows == nil || *cfg.Rows != 50 {
		t.Errorf("AutoDetect Rows = %v, want 50", cfg.Rows)
	}
}
