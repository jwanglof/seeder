package infer_test

import (
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/mickamy/seeder/internal/infer"
	"github.com/mickamy/seeder/internal/introspect"
	"github.com/mickamy/seeder/internal/matchers"
)

func TestPick_ByName(t *testing.T) {
	t.Parallel()

	atSign := matchers.Match(regexp.MustCompile(`@`))
	httpURL := matchers.MatchPrefix("http")
	nonEmpty := matchers.NonEmpty()

	cases := []struct {
		col   string
		kind  introspect.Kind
		check matchers.Matcher
	}{
		{"email", introspect.KindString, atSign},
		{"user_email", introspect.KindString, atSign},
		{"first_name", introspect.KindString, nonEmpty},
		{"last_name", introspect.KindString, nonEmpty},
		{"name", introspect.KindString, nonEmpty},
		{"display_name", introspect.KindString, nonEmpty},
		{"phone", introspect.KindString, nonEmpty},
		{"tel", introspect.KindString, nonEmpty},
		{"avatar_url", introspect.KindString, httpURL},
		{"image_url", introspect.KindString, httpURL},
		{"homepage", introspect.KindString, httpURL},
		{"address", introspect.KindString, nonEmpty},
		{"city", introspect.KindString, nonEmpty},
		{"country", introspect.KindString, nonEmpty},
		{"zip", introspect.KindString, nonEmpty},
		{"description", introspect.KindString, nonEmpty},
		{"bio", introspect.KindString, nonEmpty},
		{"title", introspect.KindString, nonEmpty},
		{"subject", introspect.KindString, nonEmpty},
		{"created_at", introspect.KindTimestamp, matchers.Type[time.Time]()},
		{"updated_at", introspect.KindTimestamp, matchers.Type[time.Time]()},
		{"birthday", introspect.KindDate, matchers.Type[time.Time]()},
		{"age", introspect.KindInt, matchers.Type[int]()},
		{"price", introspect.KindInt, matchers.Type[int]()},
		{"amount", introspect.KindInt, matchers.Type[int]()},
		{"quantity", introspect.KindInt, matchers.Type[int]()},
		{"is_active", introspect.KindBool, matchers.Type[bool]()},
		{"has_subscription", introspect.KindBool, matchers.Type[bool]()},
		{"verified_flag", introspect.KindBool, matchers.Type[bool]()},
	}

	for _, tc := range cases {
		t.Run(tc.col, func(t *testing.T) {
			t.Parallel()
			f := gofakeit.New(42)
			gen := infer.Pick(f, introspect.Column{Name: tc.col, Kind: tc.kind}, nil, infer.LocaleEN)
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
	gen := infer.Pick(f, col, nil, infer.LocaleEN)
	matchers.Type[string]()(t, gen())
}

func TestPick_FallsBackToKind(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	col := introspect.Column{
		Name:     "totally_unknown_xyz",
		DataType: "integer",
		Kind:     introspect.KindInt,
	}
	gen := infer.Pick(f, col, nil, infer.LocaleEN)
	matchers.Type[int]()(t, gen())
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
	gen := infer.Pick(f, col, enums, infer.LocaleEN)
	matchers.Member([]string{"pending", "paid", "shipped"})(t, gen())
}

func TestPick_LocaleJA(t *testing.T) {
	t.Parallel()

	cases := []struct {
		col   string
		check matchers.Matcher
	}{
		{"first_name", expectKana},
		{"last_name", expectKana},
		{"full_name", expectKana},
		{"display_name", expectKana},
		{"phone", matchers.MatchPrefix("0")},
		{"address", expectKana},
		{"city", expectKana},
		{"prefecture", expectKana},
		{"country", matchers.Equal("日本")}, //nolint:gosmopolitan // ja-locale assertion
		{"state", expectKana},
		{"zip", matchers.Match(regexp.MustCompile(`^\d{3}-\d{4}$`))},
	}

	for _, tc := range cases {
		t.Run(tc.col, func(t *testing.T) {
			t.Parallel()
			f := gofakeit.New(42)
			gen := infer.Pick(f, introspect.Column{Name: tc.col, Kind: introspect.KindString}, nil, infer.LocaleJA)
			tc.check(t, gen())
		})
	}
}

func TestPick_LocaleJA_KeepsLocaleNeutralRules(t *testing.T) {
	t.Parallel()

	// email / url / age etc must keep returning the en (locale-neutral) value
	// even under LocaleJA, since the rule has no ja override.
	f := gofakeit.New(42)
	cases := []struct {
		col   string
		kind  introspect.Kind
		check matchers.Matcher
	}{
		{"email", introspect.KindString, matchers.Match(regexp.MustCompile(`@`))},
		{"avatar_url", introspect.KindString, matchers.MatchPrefix("http")},
		{"age", introspect.KindInt, matchers.Type[int]()},
	}
	for _, tc := range cases {
		t.Run(tc.col, func(t *testing.T) {
			t.Parallel()
			gen := infer.Pick(f, introspect.Column{Name: tc.col, Kind: tc.kind}, nil, infer.LocaleJA)
			tc.check(t, gen())
		})
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

// expectKana asserts the value contains at least one CJK/kana code point, so
// we can distinguish ja-locale output from a stray English fallback.
func expectKana(t *testing.T, v any) {
	t.Helper()
	s, ok := v.(string)
	if !ok || s == "" {
		t.Fatalf("got %v (%T); want non-empty string", v, v)
	}
	if !slices.ContainsFunc([]rune(s), isJapaneseRune) {
		t.Errorf("got %q; want at least one CJK/kana rune", s)
	}
}

func isJapaneseRune(r rune) bool {
	switch {
	case r >= 0x3040 && r <= 0x309F: // Hiragana
		return true
	case r >= 0x30A0 && r <= 0x30FF: // Katakana
		return true
	case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
		return true
	}
	return false
}
