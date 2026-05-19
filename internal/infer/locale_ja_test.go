//nolint:gosmopolitan,testpackage // tests exercise the Japanese dictionary and need access to private generators.
package infer

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/mickamy/seeder/internal/matchers"
)

func TestLocaleJA_Generators(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		fn   func(*gofakeit.Faker) any
		want matchers.Matcher
	}{
		{"lastName", lastNameJA, matchers.Member(jaLastNames)},
		{"firstName", firstNameJA, matchers.Member(jaFirstNames)},
		{"prefecture", prefectureJA, matchers.Member(jaPrefectures)},
		{"city", cityJA, matchers.Member(jaCities)},
		{"country", countryJA, matchers.Equal("日本")},
		{"fullName", fullNameJA, expectFullName},
		{"address", addressJA, expectAddress},
		{"zip", zipJA, matchers.Match(regexp.MustCompile(`^\d{3}-\d{4}$`))},
		{"phone", phoneJA, matchers.Match(regexp.MustCompile(`^0[789]0-\d{4}-\d{4}$`))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := gofakeit.New(42)
			tc.want(t, tc.fn(f))
		})
	}
}

func expectFullName(t *testing.T, v any) {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("got %T; want string", v)
	}
	parts := strings.SplitN(s, " ", 2)
	if len(parts) != 2 {
		t.Fatalf("got %q; want two parts separated by space", s)
	}
	if !slices.Contains(jaLastNames, parts[0]) {
		t.Errorf("last name %q not in dictionary", parts[0])
	}
	if !slices.Contains(jaFirstNames, parts[1]) {
		t.Errorf("first name %q not in dictionary", parts[1])
	}
}

func expectAddress(t *testing.T, v any) {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("got %T; want string", v)
	}
	pref := ""
	for _, p := range jaPrefectures {
		if strings.HasPrefix(s, p) {
			pref = p
			break
		}
	}
	if pref == "" {
		t.Fatalf("got %q; no prefecture prefix found", s)
	}
	rest := strings.TrimPrefix(s, pref)
	city := ""
	for _, c := range jaCities {
		if strings.HasPrefix(rest, c) {
			city = c
			break
		}
	}
	if city == "" {
		t.Fatalf("got %q; no city after prefecture", s)
	}
	tail := strings.TrimPrefix(rest, city)
	if !regexp.MustCompile(`^\d+-\d+-\d+$`).MatchString(tail) {
		t.Errorf("got tail %q; want N-N-N", tail)
	}
}
