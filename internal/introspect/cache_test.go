package introspect_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mickamy/seeder/internal/introspect"
)

func TestSaveAndLoadCache_Roundtrip(t *testing.T) {
	t.Parallel()

	defaultExpr := "nextval('users_id_seq'::regclass)"
	want := introspect.Schema{
		Tables: []introspect.Table{
			{
				Name: "users",
				Columns: []introspect.Column{
					{
						Name:       "id",
						DataType:   "integer",
						Kind:       introspect.KindInt,
						Default:    &defaultExpr,
						IsIdentity: false,
						IsUnique:   true,
					},
					{
						Name:     "email",
						DataType: "text",
						Kind:     introspect.KindString,
						IsUnique: true,
					},
				},
				PrimaryKey: []string{"id"},
			},
		},
	}

	path := filepath.Join(t.TempDir(), "schema.gob")
	if err := introspect.SaveCache(path, want); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	got, ok, err := introspect.LoadCache(path)
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if !ok {
		t.Fatal("LoadCache ok=false; want true")
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadCache mismatch:\n got: %+v\nwant: %+v", got, want)
	}
}

func TestLoadCache_MissingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "does-not-exist.gob")
	got, ok, err := introspect.LoadCache(path)
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if ok {
		t.Errorf("ok = true; want false (file does not exist)")
	}
	if !reflect.DeepEqual(got, introspect.Schema{}) {
		t.Errorf("got = %v; want zero Schema", got)
	}
}
