package infer

import (
	"regexp"
	"slices"
	"strings"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/mickamy/seeder/internal/generator"
	"github.com/mickamy/seeder/internal/introspect"
)

type nameRule struct {
	re    *regexp.Regexp
	kinds []introspect.Kind
	gen   func(*gofakeit.Faker) any
}

var (
	stringKinds = []introspect.Kind{introspect.KindString}
	intKinds    = []introspect.Kind{introspect.KindInt}
	moneyKinds  = []introspect.Kind{introspect.KindInt, introspect.KindFloat}
	dateKinds   = []introspect.Kind{introspect.KindDate, introspect.KindTime, introspect.KindTimestamp}
	boolKinds   = []introspect.Kind{introspect.KindBool}
)

var nameRules = []nameRule{
	{
		regexp.MustCompile(`(^|_)email(s)?$`),
		stringKinds,
		func(f *gofakeit.Faker) any { return f.Email() },
	},
	{
		regexp.MustCompile(`^first_name$|(^|_)given_name$`),
		stringKinds,
		func(f *gofakeit.Faker) any { return f.FirstName() },
	},
	{
		regexp.MustCompile(`^last_name$|^surname$|^family_name$`),
		stringKinds,
		func(f *gofakeit.Faker) any { return f.LastName() },
	},
	{
		regexp.MustCompile(`^name$|^full_name$|^username$|^user_name$|^nickname$|^display_name$`),
		stringKinds,
		func(f *gofakeit.Faker) any { return f.Name() },
	},
	{
		regexp.MustCompile(`(^|_)phone($|_number$)|^tel$|^mobile$`),
		stringKinds,
		func(f *gofakeit.Faker) any { return f.Phone() },
	},
	{
		regexp.MustCompile(`(^|_)url$|^link$|^homepage$|^website$|^avatar(_url)?$|^image(_url)?$`),
		stringKinds,
		func(f *gofakeit.Faker) any { return f.URL() },
	},
	{
		regexp.MustCompile(`^address$|^street$`),
		stringKinds,
		func(f *gofakeit.Faker) any { return f.Address().Address },
	},
	{
		regexp.MustCompile(`^city$|^town$`),
		stringKinds,
		func(f *gofakeit.Faker) any { return f.City() },
	},
	{
		regexp.MustCompile(`^country$|^prefecture$`),
		stringKinds,
		func(f *gofakeit.Faker) any { return f.Country() },
	},
	{
		regexp.MustCompile(`^state$|^region$|^province$`),
		stringKinds,
		func(f *gofakeit.Faker) any { return f.State() },
	},
	{
		regexp.MustCompile(`^zip$|^zipcode$|^postal_code$|^postcode$`),
		stringKinds,
		func(f *gofakeit.Faker) any { return f.Zip() },
	},
	{
		regexp.MustCompile(`^description$|^bio$|^comment$|^note$|^body$|^content$|^remarks?$`),
		stringKinds,
		func(f *gofakeit.Faker) any { return f.LoremIpsumParagraph(1, 5, 12, " ") },
	},
	{
		regexp.MustCompile(`^title$|^subject$|^headline$|^summary$`),
		stringKinds,
		func(f *gofakeit.Faker) any { return f.LoremIpsumSentence(10) },
	},
	{
		regexp.MustCompile(`^birthday$|^birth_date$|^dob$`),
		dateKinds,
		func(f *gofakeit.Faker) any { return f.PastDate() },
	},
	{
		regexp.MustCompile(`(^|_)age$`),
		intKinds,
		func(f *gofakeit.Faker) any { return f.Age() },
	},
	{
		regexp.MustCompile(`(^|_)at$`),
		dateKinds,
		func(f *gofakeit.Faker) any { return f.PastDate() },
	},
	{
		regexp.MustCompile(`^price$|^amount$|^cost$|^total$|(_yen|_usd|_jpy|_eur)$`),
		moneyKinds,
		func(f *gofakeit.Faker) any { return f.Number(1, 100000) },
	},
	{
		regexp.MustCompile(`^count$|^quantity$|^qty$|^num$|^num_[a-z_]+$`),
		intKinds,
		func(f *gofakeit.Faker) any { return f.Number(0, 1000) },
	},
	{
		regexp.MustCompile(`^is_[a-z_]+$|^has_[a-z_]+$|_flag$|^enabled$|^disabled$|^active$`),
		boolKinds,
		func(f *gofakeit.Faker) any { return f.Bool() },
	},
}

func Pick(f *gofakeit.Faker, col introspect.Column, enums map[string][]string) generator.Func {
	name := strings.ToLower(col.Name)
	for _, r := range nameRules {
		if !r.re.MatchString(name) {
			continue
		}
		if !slices.Contains(r.kinds, col.Kind) {
			continue
		}
		gen := r.gen
		return func() any { return gen(f) }
	}

	return generator.FromKind(f, col.Kind, col.UDTName, enums)
}
