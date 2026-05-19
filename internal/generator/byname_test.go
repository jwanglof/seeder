package generator_test

import (
	"regexp"
	"slices"
	"testing"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/mickamy/seeder/internal/generator"
	"github.com/mickamy/seeder/internal/matchers"
)

func TestByName_KnownGenerators(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		check matchers.Matcher
	}{
		{"Email", matchers.Match(regexp.MustCompile(`@`))},
		{"URL", matchers.MatchPrefix("http")},
		{"UUID", matchers.Match(regexp.MustCompile(`^[0-9a-f-]{36}$`))},
		{"Name", matchers.NonEmpty()},
		{"Bool", matchers.Type[bool]()},
		{"Word", matchers.NonEmpty()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := gofakeit.New(42)
			gen, err := generator.ByName(f, tc.name)
			if err != nil {
				t.Fatalf("ByName(%q): %v", tc.name, err)
			}
			tc.check(t, gen())
		})
	}
}

func TestByName_Unknown(t *testing.T) {
	t.Parallel()

	f := gofakeit.New(42)
	_, err := generator.ByName(f, "NotAGenerator")
	if err == nil {
		t.Fatal("ByName: nil error, want failure")
	}
}

func TestIsKnown(t *testing.T) {
	t.Parallel()

	if !generator.IsKnown("Email") {
		t.Error("IsKnown(Email) = false; want true")
	}
	if generator.IsKnown("NotAGenerator") {
		t.Error("IsKnown(NotAGenerator) = true; want false")
	}
}

func TestKnownNames_Sorted(t *testing.T) {
	t.Parallel()

	got := generator.KnownNames()
	if len(got) == 0 {
		t.Fatal("KnownNames is empty")
	}
	if !slices.IsSorted(got) {
		t.Errorf("KnownNames not sorted: %v", got)
	}
}
