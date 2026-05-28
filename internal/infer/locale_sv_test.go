package infer

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/mickamy/seeder/internal/matchers"
)

func TestLocaleSV_Generators(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		fn   func(*gofakeit.Faker) any
		want matchers.Matcher
	}{
		{"lastName", lastNameSV, matchers.Member(svLastNames)},
		{"firstName", firstNameSV, matchers.Member(svFirstNames)},
		{"county", countySV, matchers.Member(svCounties)},
		{"city", citySV, matchers.Member(svCities)},
		{"country", countrySV, matchers.Equal("Sverige")},
		{"fullName", fullNameSV, expectFullNameSV},
		{"address", addressSV, expectAddressSV},
		{"zip", zipSV, matchers.Match(regexp.MustCompile(`^\d{3} \d{2}$`))},
		{"phone", phoneSV, matchers.Match(regexp.MustCompile(`^0(70|72|73|76|79)-\d{3} \d{2} \d{2}$`))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := gofakeit.New(42)
			tc.want(t, tc.fn(f))
		})
	}
}

func expectFullNameSV(t *testing.T, v any) {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("got %T; want string", v)
	}
	parts := strings.SplitN(s, " ", 2)
	if len(parts) != 2 {
		t.Fatalf("got %q; want two parts separated by space", s)
	}
	if !slices.Contains(svFirstNames, parts[0]) {
		t.Errorf("first name %q not in dictionary", parts[0])
	}
	if !slices.Contains(svLastNames, parts[1]) {
		t.Errorf("last name %q not in dictionary", parts[1])
	}
}

func expectAddressSV(t *testing.T, v any) {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("got %T; want string", v)
	}

	// Split into "{street} {number}" and "{zip} {city}".
	left, right, found := strings.Cut(s, ", ")
	if !found {
		t.Fatalf("got %q; want comma-separated street and locality", s)
	}

	// Right side: "NNN NN City" — three digits, space, two digits, space, city.
	rightRe := regexp.MustCompile(`^(\d{3} \d{2}) (.+)$`)
	rightMatch := rightRe.FindStringSubmatch(right)
	if rightMatch == nil {
		t.Fatalf("got locality %q; want \"NNN NN City\"", right)
	}
	city := rightMatch[2]
	if !slices.Contains(svCities, city) {
		t.Errorf("city %q not in dictionary", city)
	}

	// Left side: street name followed by " <number>". Match against the known
	// street dictionary since street names can contain spaces (e.g.,
	// "Birger Jarlsgatan").
	street := ""
	for _, candidate := range svStreetNames {
		prefix := candidate + " "
		if strings.HasPrefix(left, prefix) {
			street = candidate
			break
		}
	}
	if street == "" {
		t.Fatalf("got %q; no street prefix found", left)
	}
	numStr := strings.TrimPrefix(left, street+" ")
	if !regexp.MustCompile(`^\d+$`).MatchString(numStr) {
		t.Errorf("got street number %q; want digits", numStr)
	}
}
