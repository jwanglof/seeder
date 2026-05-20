package infer

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/mickamy/seeder/internal/generator"
	"github.com/mickamy/seeder/internal/introspect"
)

type localeGen func(*gofakeit.Faker) any

// Shared with uniqueWrap so the email / image shapes stay valid under UNIQUE.
var (
	emailNameRe = regexp.MustCompile(`(^|_)email(s)?$`)
	imageNameRe = regexp.MustCompile(`^avatar(_url)?$|^image(_url)?$|^photo(_url)?$|^picture(_url)?$|^thumbnail(_url)?$`)
)

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
		re:    emailNameRe,
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
		label: "Image",
		re:    imageNameRe,
		kinds: stringKinds,
		gens: map[Locale]localeGen{
			LocaleEN: func(f *gofakeit.Faker) any {
				return fmt.Sprintf("https://picsum.photos/seed/%d/200/200", f.Number(1, 1_000_000))
			},
		},
	},
	{
		label: "URL",
		re:    regexp.MustCompile(`(^|_)url$|^link$|^homepage$|^website$`),
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
	base := pickBase(f, col, locale)
	if !col.IsUnique {
		return base
	}

	return uniqueWrap(f, col, base)
}

func pickBase(f *gofakeit.Faker, col introspect.Column, locale Locale) generator.Func {
	if r, ok := matchRule(col); ok {
		gen := r.gen(locale)

		return func() any { return gen(f) }
	}

	return generator.FromKind(f, col.Kind, col.EnumValues)
}

func uniqueIntStart(f *gofakeit.Faker, dataType string) int {
	switch strings.ToLower(dataType) {
	case "tinyint", "smallint":
		return 1
	}

	return f.Number(1, 1000)
}

// uniqueWrap is best-effort: large row counts can still collide and surface as
// a unique-violation from the DB.
func uniqueWrap(f *gofakeit.Faker, col introspect.Column, base generator.Func) generator.Func {
	name := strings.ToLower(col.Name)
	switch col.Kind {
	case introspect.KindString:
		if emailNameRe.MatchString(name) {
			return func() any { return f.UUID() + "@example.com" }
		}
		if imageNameRe.MatchString(name) {
			// Vary the picsum seed segment so the URL stays valid as
			// `/seed/<uuid>/200/200` instead of being suffixed and breaking
			// the path shape.
			return func() any {
				return fmt.Sprintf("https://picsum.photos/seed/%s/200/200", f.UUID())
			}
		}

		return func() any {
			v, ok := base().(string)
			if !ok {
				return f.UUID()
			}

			return v + "-" + f.UUID()
		}
	case introspect.KindInt:
		// Counter-based to give every row a distinct value. Narrow integer
		// types (tinyint / smallint) start from 1 so the small cardinality is
		// not wasted on an offset; wider types take a random offset so reseed
		// values do not align with PKs from a previous run.
		counter := uniqueIntStart(f, col.DataType)

		return func() any {
			v := counter
			counter++

			return v
		}
	case introspect.KindUnknown,
		introspect.KindBool,
		introspect.KindFloat,
		introspect.KindUUID,
		introspect.KindDate,
		introspect.KindTime,
		introspect.KindTimestamp,
		introspect.KindJSON,
		introspect.KindEnum,
		introspect.KindBytes:
		return base
	}

	return base
}

// Explain reports which rule Pick will use for col; meant for --verbose output.
func Explain(col introspect.Column) string {
	prefix := ""
	if col.IsUnique {
		prefix = "unique-aware "
	}
	if r, ok := matchRule(col); ok {
		return prefix + "name match: " + r.label
	}
	if col.Kind == introspect.KindEnum && len(col.EnumValues) > 0 {
		return prefix + "enum: " + strings.Join(col.EnumValues, ",")
	}

	return prefix + "kind: " + col.Kind.String()
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
