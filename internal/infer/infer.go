package infer

import (
	"regexp"
	"slices"
	"strings"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/mickamy/seeder/internal/generator"
	"github.com/mickamy/seeder/internal/introspect"
)

type localeGen func(*gofakeit.Faker) any

type nameRule struct {
	label string
	re    *regexp.Regexp
	kinds []introspect.Kind
	// gens maps each supported locale to its generator. LocaleEN must always
	// be present and acts as the fallback for locales without an entry.
	gens map[Locale]localeGen
}

func (r nameRule) gen(locale Locale) localeGen {
	if g, ok := r.gens[locale]; ok {
		return g
	}
	return r.gens[LocaleEN]
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
		label: "Email",
		re:    regexp.MustCompile(`(^|_)email(s)?$`),
		kinds: stringKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.Email() },
		},
	},
	{
		label: "FirstName",
		re:    regexp.MustCompile(`^first_name$|(^|_)given_name$`),
		kinds: stringKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.FirstName() },
			LocaleJA: firstNameJA,
		},
	},
	{
		label: "LastName",
		re:    regexp.MustCompile(`^last_name$|^surname$|^family_name$`),
		kinds: stringKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.LastName() },
			LocaleJA: lastNameJA,
		},
	},
	{
		label: "Name",
		re:    regexp.MustCompile(`^name$|^full_name$|^username$|^user_name$|^nickname$|^display_name$`),
		kinds: stringKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.Name() },
			LocaleJA: fullNameJA,
		},
	},
	{
		label: "Phone",
		re:    regexp.MustCompile(`(^|_)phone($|_number$)|^tel$|^mobile$`),
		kinds: stringKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.Phone() },
			LocaleJA: phoneJA,
		},
	},
	{
		label: "URL",
		re:    regexp.MustCompile(`(^|_)url$|^link$|^homepage$|^website$|^avatar(_url)?$|^image(_url)?$`),
		kinds: stringKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.URL() },
		},
	},
	{
		label: "Address",
		re:    regexp.MustCompile(`^address$|^street$`),
		kinds: stringKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.Address().Address },
			LocaleJA: addressJA,
		},
	},
	{
		label: "City",
		re:    regexp.MustCompile(`^city$|^town$`),
		kinds: stringKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.City() },
			LocaleJA: cityJA,
		},
	},
	{
		label: "Country",
		re:    regexp.MustCompile(`^country$`),
		kinds: stringKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.Country() },
			LocaleJA: countryJA,
		},
	},
	{
		label: "State",
		re:    regexp.MustCompile(`^state$|^region$|^province$|^prefecture$`),
		kinds: stringKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.State() },
			LocaleJA: prefectureJA,
		},
	},
	{
		label: "Zip",
		re:    regexp.MustCompile(`^zip$|^zipcode$|^postal_code$|^postcode$`),
		kinds: stringKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.Zip() },
			LocaleJA: zipJA,
		},
	},
	{
		label: "LoremIpsumParagraph",
		re:    regexp.MustCompile(`^description$|^bio$|^comment$|^note$|^body$|^content$|^remarks?$`),
		kinds: stringKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.LoremIpsumParagraph(1, 5, 12, " ") },
		},
	},
	{
		label: "LoremIpsumSentence",
		re:    regexp.MustCompile(`^title$|^subject$|^headline$|^summary$`),
		kinds: stringKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.LoremIpsumSentence(10) },
		},
	},
	{
		label: "Birthday",
		re:    regexp.MustCompile(`^birthday$|^birth_date$|^dob$`),
		kinds: dateKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.PastDate() },
		},
	},
	{
		label: "Age",
		re:    regexp.MustCompile(`(^|_)age$`),
		kinds: intKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.Age() },
		},
	},
	{
		label: "PastDate",
		re:    regexp.MustCompile(`(^|_)at$`),
		kinds: dateKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.PastDate() },
		},
	},
	{
		label: "Money",
		re:    regexp.MustCompile(`^price$|^amount$|^cost$|^total$|(_yen|_usd|_jpy|_eur)$`),
		kinds: moneyKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.Number(1, 100000) },
		},
	},
	{
		label: "Count",
		re:    regexp.MustCompile(`^count$|^quantity$|^qty$|^num$|^num_[a-z_]+$`),
		kinds: intKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.Number(0, 1000) },
		},
	},
	{
		label: "Bool",
		re:    regexp.MustCompile(`^is_[a-z_]+$|^has_[a-z_]+$|_flag$|^enabled$|^disabled$|^active$`),
		kinds: boolKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any { return f.Bool() },
		},
	},
}

func Pick(f *gofakeit.Faker, col introspect.Column, locale Locale) generator.Func {
	if r, ok := matchRule(col); ok {
		gen := r.gen(locale)

		return func() any { return gen(f) }
	}

	return generator.FromKind(f, col.Kind, col.EnumValues)
}

// Explain reports which rule Pick will use for col; meant for --verbose output.
func Explain(col introspect.Column) string {
	if r, ok := matchRule(col); ok {
		return "name match: " + r.label
	}
	if col.Kind == introspect.KindEnum && len(col.EnumValues) > 0 {
		return "enum: " + strings.Join(col.EnumValues, ",")
	}

	return "kind: " + col.Kind.String()
}

func matchRule(col introspect.Column) (nameRule, bool) {
	name := strings.ToLower(col.Name)
	for _, r := range nameRules {
		if !r.re.MatchString(name) {
			continue
		}
		if !slices.Contains(r.kinds, col.Kind) {
			continue
		}

		return r, true
	}

	return nameRule{}, false
}
