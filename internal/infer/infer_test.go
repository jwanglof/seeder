package infer_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
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
	picsumURL := matchers.MatchPrefix("https://picsum.photos/")
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
		{"avatar_url", introspect.KindString, picsumURL},
		{"image_url", introspect.KindString, picsumURL},
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
			gen := infer.Pick(f, introspect.Column{Name: tc.col, Kind: tc.kind}, infer.LocaleEN)
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
	gen := infer.Pick(f, col, infer.LocaleEN)
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
	gen := infer.Pick(f, col, infer.LocaleEN)
	matchers.Type[int]()(t, gen())
}

func TestPick_UniqueEmail(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	col := introspect.Column{Name: "email", Kind: introspect.KindString, IsUnique: true}
	gen := infer.Pick(f, col, infer.LocaleEN)

	seen := make(map[string]bool, 1000)
	for range 1000 {
		v := gen()
		s, ok := v.(string)
		if !ok {
			t.Fatalf("unique email value = %T; want string", v)
		}
		if !strings.HasSuffix(s, "@example.com") {
			t.Errorf("unique email %q does not end with @example.com", s)
		}
		if seen[s] {
			t.Fatalf("unique email collided after few samples: %s", s)
		}
		seen[s] = true
	}
}

func TestPick_UniqueString(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	col := introspect.Column{Name: "title", Kind: introspect.KindString, IsUnique: true}
	gen := infer.Pick(f, col, infer.LocaleEN)

	seen := make(map[string]bool, 1000)
	for range 1000 {
		v := gen()
		s, ok := v.(string)
		if !ok {
			t.Fatalf("unique string value = %T; want string", v)
		}
		if !strings.Contains(s, "-") {
			t.Errorf("unique string %q has no UUID suffix", s)
		}
		if seen[s] {
			t.Fatalf("unique string collided after few samples: %s", s)
		}
		seen[s] = true
	}
}

func TestPick_UniqueImage(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	col := introspect.Column{Name: "avatar_url", Kind: introspect.KindString, IsUnique: true}
	gen := infer.Pick(f, col, infer.LocaleEN)

	seen := make(map[string]bool, 1000)
	for range 1000 {
		v := gen()
		s, ok := v.(string)
		if !ok {
			t.Fatalf("unique image value = %T; want string", v)
		}
		if !strings.HasPrefix(s, "https://picsum.photos/seed/") || !strings.HasSuffix(s, "/200/200") {
			t.Errorf("unique image %q does not look like picsum seed URL", s)
		}
		if seen[s] {
			t.Fatalf("unique image collided after few samples: %s", s)
		}
		seen[s] = true
	}
}

func TestPick_UniqueInt(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	col := introspect.Column{Name: "code", Kind: introspect.KindInt, IsUnique: true}
	gen := infer.Pick(f, col, infer.LocaleEN)

	const samples = 1000
	seen := make(map[int]bool, samples)
	maxSeen := 0
	for range samples {
		v := gen()
		n, ok := v.(int)
		if !ok {
			t.Fatalf("unique int value = %T; want int", v)
		}
		if seen[n] {
			t.Fatalf("unique int collided: %d", n)
		}
		seen[n] = true
		if n > maxSeen {
			maxSeen = n
		}
	}
	// 1000 samples from an offset in [1, 1000] should fit in any signed
	// 16-bit (smallint) column.
	if maxSeen >= 32768 {
		t.Errorf("unique int max = %d; want < 32768 (smallint-safe)", maxSeen)
	}
}

func TestPick_UniqueInt_NarrowType(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	col := introspect.Column{
		Name:     "code",
		Kind:     introspect.KindInt,
		DataType: "tinyint",
		IsUnique: true,
	}
	gen := infer.Pick(f, col, infer.LocaleEN)

	// Narrow ints start at 1 and increment, so the small cardinality of
	// tinyint (signed max 127) is fully usable.
	for i := 1; i <= 50; i++ {
		v := gen()
		n, ok := v.(int)
		if !ok {
			t.Fatalf("value = %T; want int", v)
		}
		if n != i {
			t.Errorf("step %d: value = %d; want %d", i, n, i)
		}
	}
}

func TestPick_UniqueString_RespectsMaxLength_Phone(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	col := introspect.Column{
		Name:      "tel",
		Kind:      introspect.KindString,
		IsUnique:  true,
		MaxLength: 11,
	}
	gen := infer.Pick(f, col, infer.LocaleEN)

	seen := make(map[string]bool, 1000)
	for i := range 1000 {
		v := gen()
		s, ok := v.(string)
		if !ok {
			t.Fatalf("step %d: value = %T; want string", i, v)
		}
		if len(s) > col.MaxLength {
			t.Fatalf("step %d: value %q length %d exceeds MaxLength %d", i, s, len(s), col.MaxLength)
		}
		if seen[s] {
			t.Fatalf("step %d: value collided: %s", i, s)
		}
		seen[s] = true
	}
}

func TestPick_UniqueEmail_RespectsMaxLength(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	col := introspect.Column{
		Name:      "email",
		Kind:      introspect.KindString,
		IsUnique:  true,
		MaxLength: 36,
	}
	gen := infer.Pick(f, col, infer.LocaleEN)

	seen := make(map[string]bool, 500)
	for i := range 500 {
		v := gen()
		s, ok := v.(string)
		if !ok {
			t.Fatalf("step %d: value = %T; want string", i, v)
		}
		if len(s) > col.MaxLength {
			t.Fatalf("step %d: value %q length %d exceeds MaxLength %d", i, s, len(s), col.MaxLength)
		}
		if !strings.HasSuffix(s, "@example.com") {
			t.Errorf("step %d: value %q lost @example.com suffix", i, s)
		}
		if seen[s] {
			t.Fatalf("step %d: value collided: %s", i, s)
		}
		seen[s] = true
	}
}

func TestPick_UniqueImage_RespectsMaxLength(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	col := introspect.Column{
		Name:      "avatar_url",
		Kind:      introspect.KindString,
		IsUnique:  true,
		MaxLength: 50,
	}
	gen := infer.Pick(f, col, infer.LocaleEN)

	seen := make(map[string]bool, 500)
	for i := range 500 {
		v := gen()
		s, ok := v.(string)
		if !ok {
			t.Fatalf("step %d: value = %T; want string", i, v)
		}
		if len(s) > col.MaxLength {
			t.Fatalf("step %d: value %q length %d exceeds MaxLength %d", i, s, len(s), col.MaxLength)
		}
		if !strings.HasPrefix(s, "https://picsum.photos/seed/") || !strings.HasSuffix(s, "/200/200") {
			t.Errorf("step %d: value %q lost picsum URL skeleton", i, s)
		}
		if seen[s] {
			t.Fatalf("step %d: value collided: %s", i, s)
		}
		seen[s] = true
	}
}

func TestPick_UniqueString_VeryTight_CounterShape(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	col := introspect.Column{
		Name:      "code",
		Kind:      introspect.KindString,
		IsUnique:  true,
		MaxLength: 5,
	}
	gen := infer.Pick(f, col, infer.LocaleEN)

	for i := 1; i <= 100; i++ {
		v := gen()
		s, ok := v.(string)
		if !ok {
			t.Fatalf("step %d: value = %T; want string", i, v)
		}
		if got, want := s, fmt.Sprintf("%05d", i); got != want {
			t.Errorf("step %d: value = %q; want %q", i, got, want)
		}
	}
}

func TestPick_EnumByKind(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	col := introspect.Column{
		Name:       "status",
		DataType:   "USER-DEFINED",
		UDTName:    "order_status",
		EnumValues: []string{"pending", "paid", "shipped"},
		Kind:       introspect.KindEnum,
	}
	gen := infer.Pick(f, col, infer.LocaleEN)
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
		{"prefecture", matchers.Match(regexp.MustCompile(`(都|道|府|県)$`))}, //nolint:gosmopolitan
		{"country", matchers.Equal("日本")},                                //nolint:gosmopolitan
		{"state", expectKana},
		{"zip", matchers.Match(regexp.MustCompile(`^\d{3}-\d{4}$`))},
	}

	for _, tc := range cases {
		t.Run(tc.col, func(t *testing.T) {
			t.Parallel()
			f := gofakeit.New(42)
			gen := infer.Pick(f, introspect.Column{Name: tc.col, Kind: introspect.KindString}, infer.LocaleJA)
			tc.check(t, gen())
		})
	}
}

func TestPick_LocaleJA_KeepsLocaleNeutralRules(t *testing.T) {
	t.Parallel()

	// email / url / age etc must keep returning the en (locale-neutral) value
	// even under LocaleJA, since the rule has no ja override.
	cases := []struct {
		col   string
		kind  introspect.Kind
		check matchers.Matcher
	}{
		{"email", introspect.KindString, matchers.Match(regexp.MustCompile(`@`))},
		{"avatar_url", introspect.KindString, matchers.MatchPrefix("https://picsum.photos/")},
		{"age", introspect.KindInt, matchers.Type[int]()},
	}
	for _, tc := range cases {
		t.Run(tc.col, func(t *testing.T) {
			t.Parallel()
			f := gofakeit.New(42)
			gen := infer.Pick(f, introspect.Column{Name: tc.col, Kind: tc.kind}, infer.LocaleJA)
			tc.check(t, gen())
		})
	}
}

func TestExplain(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		col  introspect.Column
		want string
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
			name: "enum with labels",
			col:  introspect.Column{Name: "status", Kind: introspect.KindEnum, EnumValues: []string{"a", "b"}},
			want: "enum: a,b",
		},
		{
			name: "enum without labels falls back to kind",
			col:  introspect.Column{Name: "status", Kind: introspect.KindEnum},
			want: "kind: enum",
		},
		{
			name: "unknown column falls back to kind",
			col:  introspect.Column{Name: "totally_unknown_xyz", Kind: introspect.KindInt},
			want: "kind: int",
		},
		{
			name: "unique-aware name match",
			col:  introspect.Column{Name: "email", Kind: introspect.KindString, IsUnique: true},
			want: "unique-aware name match: Email",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := infer.Explain(tc.col); got != tc.want {
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
