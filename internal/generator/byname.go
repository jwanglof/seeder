package generator

import (
	"fmt"
	"slices"
	"strings"

	"github.com/brianvoe/gofakeit/v7"
)

type byNameFn func(*gofakeit.Faker) any

var byName = map[string]byNameFn{
	"Bool":       func(f *gofakeit.Faker) any { return f.Bool() },
	"City":       func(f *gofakeit.Faker) any { return f.City() },
	"Country":    func(f *gofakeit.Faker) any { return f.Country() },
	"Email":      func(f *gofakeit.Faker) any { return f.Email() },
	"FirstName":  func(f *gofakeit.Faker) any { return f.FirstName() },
	"FutureDate": func(f *gofakeit.Faker) any { return f.FutureDate() },
	"LastName":   func(f *gofakeit.Faker) any { return f.LastName() },
	"Name":       func(f *gofakeit.Faker) any { return f.Name() },
	"Paragraph":  func(f *gofakeit.Faker) any { return f.LoremIpsumParagraph(1, 5, 12, " ") },
	"PastDate":   func(f *gofakeit.Faker) any { return f.PastDate() },
	"Phone":      func(f *gofakeit.Faker) any { return f.Phone() },
	"Sentence":   func(f *gofakeit.Faker) any { return f.LoremIpsumSentence(10) },
	"State":      func(f *gofakeit.Faker) any { return f.State() },
	"URL":        func(f *gofakeit.Faker) any { return f.URL() },
	"UUID":       func(f *gofakeit.Faker) any { return f.UUID() },
	"Word":       func(f *gofakeit.Faker) any { return f.Word() },
	"Zip":        func(f *gofakeit.Faker) any { return f.Zip() },
}

func ByName(f *gofakeit.Faker, name string) (Func, error) {
	fn, ok := byName[name]
	if !ok {
		return nil, fmt.Errorf("unknown generator %q (known: %s)", name, strings.Join(KnownNames(), ", "))
	}
	return func() any { return fn(f) }, nil
}

func IsKnown(name string) bool {
	_, ok := byName[name]
	return ok
}

func KnownNames() []string {
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}
