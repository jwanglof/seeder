package generator_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/mickamy/seeder/internal/generator"
	"github.com/mickamy/seeder/internal/introspect"
)

func TestFromKind(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		kind     introspect.Kind
		wantKind string
	}{
		{"bool", introspect.KindBool, "bool"},
		{"int", introspect.KindInt, "int"},
		{"float", introspect.KindFloat, "float64"},
		{"string", introspect.KindString, "string"},
		{"uuid", introspect.KindUUID, "string"},
		{"date", introspect.KindDate, "time"},
		{"time", introspect.KindTime, "time"},
		{"timestamp", introspect.KindTimestamp, "time"},
		{"json", introspect.KindJSON, "string"},
		{"unknown_fallback", introspect.KindUnknown, "string"},
		{"bytes_fallback", introspect.KindBytes, "string"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := gofakeit.New(42)
			gen := generator.FromKind(f, tc.kind, "", nil)
			v := gen()
			if kindOf(v) != tc.wantKind {
				t.Errorf("FromKind(%v) returned %T; want kind %s", tc.kind, v, tc.wantKind)
			}
		})
	}
}

func TestFromKind_Enum(t *testing.T) {
	t.Parallel()

	enums := map[string][]string{
		"order_status": {"pending", "paid", "shipped"},
	}
	f := gofakeit.New(42)
	gen := generator.FromKind(f, introspect.KindEnum, "order_status", enums)

	for range 200 {
		v := gen()
		s, ok := v.(string)
		if !ok {
			t.Fatalf("enum value type = %T; want string", v)
		}
		if !slices.Contains(enums["order_status"], s) {
			t.Errorf("value %q not in enum labels %v", s, enums["order_status"])
		}
	}
}

func TestFromKind_UnknownEnum(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	gen := generator.FromKind(f, introspect.KindEnum, "missing_enum", nil)
	if v := gen(); v != nil {
		t.Errorf("unknown enum = %v; want nil", v)
	}
}

func TestFromKind_JSONIsValid(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	gen := generator.FromKind(f, introspect.KindJSON, "", nil)
	for range 20 {
		v := gen()
		s, ok := v.(string)
		if !ok {
			t.Fatalf("JSON value type = %T; want string", v)
		}
		var parsed any
		if err := json.Unmarshal([]byte(s), &parsed); err != nil {
			t.Errorf("JSON output is not valid JSON: %q (%v)", s, err)
		}
	}
}

func TestFromKind_DeterministicWithSeed(t *testing.T) {
	t.Parallel()

	f1 := gofakeit.New(42)
	f2 := gofakeit.New(42)
	g1 := generator.FromKind(f1, introspect.KindInt, "", nil)
	g2 := generator.FromKind(f2, introspect.KindInt, "", nil)

	for range 100 {
		if g1() != g2() {
			t.Fatalf("same seed produced different sequences")
		}
	}
}

func kindOf(v any) string {
	switch v.(type) {
	case bool:
		return "bool"
	case int:
		return "int"
	case float64:
		return "float64"
	case string:
		return "string"
	case time.Time:
		return "time"
	default:
		return "other"
	}
}
