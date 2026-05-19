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
		col      string
		dataType string
		check    func(t *testing.T, v any)
	}{
		{"email", "text", expectEmail},
		{"user_email", "text", expectEmail},
		{"first_name", "text", expectNonEmptyString},
		{"last_name", "text", expectNonEmptyString},
		{"name", "text", expectNonEmptyString},
		{"display_name", "text", expectNonEmptyString},
		{"phone", "text", expectNonEmptyString},
		{"tel", "text", expectNonEmptyString},
		{"avatar_url", "text", expectURL},
		{"image_url", "text", expectURL},
		{"homepage", "text", expectURL},
		{"address", "text", expectNonEmptyString},
		{"city", "text", expectNonEmptyString},
		{"country", "text", expectNonEmptyString},
		{"zip", "text", expectNonEmptyString},
		{"description", "text", expectNonEmptyString},
		{"bio", "text", expectNonEmptyString},
		{"title", "text", expectNonEmptyString},
		{"subject", "text", expectNonEmptyString},
		{"created_at", "timestamp", expectTime},
		{"updated_at", "timestamp", expectTime},
		{"birthday", "date", expectTime},
		{"price", "integer", expectInt},
		{"amount", "integer", expectInt},
		{"quantity", "integer", expectInt},
		{"is_active", "boolean", expectBool},
		{"has_subscription", "boolean", expectBool},
		{"verified_flag", "boolean", expectBool},
	}

	for _, tc := range cases {
		t.Run(tc.col, func(t *testing.T) {
			t.Parallel()
			f := gofakeit.New(42)
			gen := infer.Pick(f, introspect.Column{Name: tc.col, DataType: tc.dataType}, nil)
			tc.check(t, gen())
		})
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
