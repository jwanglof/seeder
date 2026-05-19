package infer_test

import (
	"strings"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/mickamy/seeder/internal/infer"
	"github.com/mickamy/seeder/internal/introspect"
)

func TestPick_ByName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		col   string
		kind  introspect.Kind
		check func(t *testing.T, v any)
	}{
		{"email", introspect.KindString, expectEmail},
		{"user_email", introspect.KindString, expectEmail},
		{"first_name", introspect.KindString, expectNonEmptyString},
		{"last_name", introspect.KindString, expectNonEmptyString},
		{"name", introspect.KindString, expectNonEmptyString},
		{"display_name", introspect.KindString, expectNonEmptyString},
		{"phone", introspect.KindString, expectNonEmptyString},
		{"tel", introspect.KindString, expectNonEmptyString},
		{"avatar_url", introspect.KindString, expectURL},
		{"image_url", introspect.KindString, expectURL},
		{"homepage", introspect.KindString, expectURL},
		{"address", introspect.KindString, expectNonEmptyString},
		{"city", introspect.KindString, expectNonEmptyString},
		{"country", introspect.KindString, expectNonEmptyString},
		{"zip", introspect.KindString, expectNonEmptyString},
		{"description", introspect.KindString, expectNonEmptyString},
		{"bio", introspect.KindString, expectNonEmptyString},
		{"title", introspect.KindString, expectNonEmptyString},
		{"subject", introspect.KindString, expectNonEmptyString},
		{"created_at", introspect.KindTimestamp, expectTime},
		{"updated_at", introspect.KindTimestamp, expectTime},
		{"birthday", introspect.KindDate, expectTime},
		{"age", introspect.KindInt, expectInt},
		{"price", introspect.KindInt, expectInt},
		{"amount", introspect.KindInt, expectInt},
		{"quantity", introspect.KindInt, expectInt},
		{"is_active", introspect.KindBool, expectBool},
		{"has_subscription", introspect.KindBool, expectBool},
		{"verified_flag", introspect.KindBool, expectBool},
	}

	for _, tc := range cases {
		t.Run(tc.col, func(t *testing.T) {
			t.Parallel()
			f := gofakeit.New(42)
			gen := infer.Pick(f, introspect.Column{Name: tc.col, Kind: tc.kind}, nil)
			tc.check(t, gen())
		})
	}
}

func TestPick_KindMismatchFallsBack(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	// `age` matches the name rule but the column is text — the rule should be
	// skipped and the generator should fall back to KindString.
	col := introspect.Column{Name: "age", Kind: introspect.KindString}
	gen := infer.Pick(f, col, nil)
	v := gen()
	if _, ok := v.(string); !ok {
		t.Errorf("age text fallback = %T; want string", v)
	}
}

func TestPick_FallsBackToKind(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	col := introspect.Column{
		Name:     "totally_unknown_xyz",
		DataType: "integer",
		Kind:     introspect.KindInt,
	}
	gen := infer.Pick(f, col, nil)
	v := gen()
	if _, ok := v.(int); !ok {
		t.Errorf("fallback for KindInt = %T; want int", v)
	}
}

func TestPick_EnumByKind(t *testing.T) {
	t.Parallel()

	enums := map[string][]string{"order_status": {"pending", "paid", "shipped"}}
	f := gofakeit.New(42)
	col := introspect.Column{
		Name:     "status",
		DataType: "USER-DEFINED",
		UDTName:  "order_status",
		Kind:     introspect.KindEnum,
	}
	gen := infer.Pick(f, col, enums)
	v := gen()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("enum value type = %T; want string", v)
	}
	if s != "pending" && s != "paid" && s != "shipped" {
		t.Errorf("got %q; not in enum", s)
	}
}

func expectEmail(t *testing.T, v any) {
	t.Helper()
	s, ok := v.(string)
	if !ok || !strings.Contains(s, "@") {
		t.Errorf("got %v (%T); want email-like string", v, v)
	}
}

func expectURL(t *testing.T, v any) {
	t.Helper()
	s, ok := v.(string)
	if !ok || (!strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://")) {
		t.Errorf("got %v (%T); want URL-like string", v, v)
	}
}

func expectNonEmptyString(t *testing.T, v any) {
	t.Helper()
	s, ok := v.(string)
	if !ok || s == "" {
		t.Errorf("got %v (%T); want non-empty string", v, v)
	}
}

func expectTime(t *testing.T, v any) {
	t.Helper()
	if _, ok := v.(time.Time); !ok {
		t.Errorf("got %T; want time.Time", v)
	}
}

func expectInt(t *testing.T, v any) {
	t.Helper()
	if _, ok := v.(int); !ok {
		t.Errorf("got %T; want int", v)
	}
}

func expectBool(t *testing.T, v any) {
	t.Helper()
	if _, ok := v.(bool); !ok {
		t.Errorf("got %T; want bool", v)
	}
}

func TestExplain(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		col   introspect.Column
		enums map[string][]string
		want  string
	}{
		{
			name: "name match",
			col:  introspect.Column{Name: "email", Kind: introspect.KindString},
			want: "name match: Email",
		},
		{
			name: "kind mismatch falls back to kind",
			col:  introspect.Column{Name: "age", Kind: introspect.KindString},
			want: "kind: string",
		},
		{
			name:  "enum with known UDT",
			col:   introspect.Column{Name: "status", Kind: introspect.KindEnum, UDTName: "order_status"},
			enums: map[string][]string{"order_status": {"a", "b"}},
			want:  "enum: order_status",
		},
		{
			name: "enum without known UDT falls back to kind",
			col:  introspect.Column{Name: "status", Kind: introspect.KindEnum},
			want: "kind: enum",
		},
		{
			name: "unknown column falls back to kind",
			col:  introspect.Column{Name: "totally_unknown_xyz", Kind: introspect.KindInt},
			want: "kind: int",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := infer.Explain(tc.col, tc.enums); got != tc.want {
				t.Errorf("Explain = %q; want %q", got, tc.want)
			}
		})
	}
}
