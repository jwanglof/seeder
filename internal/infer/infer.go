package infer

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

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
	if col.Kind == introspect.KindString && col.MaxLength > 0 {
		base = trimStringToMaxLength(base, col.MaxLength)
	}
	if !col.IsUnique {
		return base
	}

	return uniqueWrap(f, col, base)
}

// trimStringToMaxLength wraps a string generator so its output never exceeds
// maxLen characters. Trimming happens on rune boundaries so multi-byte UTF-8
// values (e.g., ja-locale names) stay valid. Non-string outputs pass through
// unchanged — only string generators can overflow a varchar(N) declaration.
//
// A single pass over the string is enough: range yields the byte index of
// each rune start, so once we have walked maxLen runes we slice the original
// string at the next rune's byte index. No []rune allocation and no extra
// RuneCountInString traversal — meaningful when seed generators emit
// paragraph-sized output millions of times.
func trimStringToMaxLength(g generator.Func, maxLen int) generator.Func {
	return func() any {
		v := g()
		s, ok := v.(string)
		if !ok {
			return v
		}
		count := 0
		for i := range s {
			if count == maxLen {
				return s[:i]
			}
			count++
		}

		return s
	}
}

func pickBase(f *gofakeit.Faker, col introspect.Column, locale Locale) generator.Func {
	if r, ok := matchRule(col); ok {
		gen := r.gen(locale)

		return func() any { return gen(f) }
	}
	if col.Kind == introspect.KindInt {
		return generator.IntForColumn(f, col.DataType)
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
// a unique-violation from the DB. When the column declares a MaxLength, each
// string strategy trims its output to fit, and very tight widths fall back to
// a zero-padded numeric counter so the value still parses on the DB side.
func uniqueWrap(f *gofakeit.Faker, col introspect.Column, base generator.Func) generator.Func {
	name := strings.ToLower(col.Name)
	switch col.Kind {
	case introspect.KindString:
		if emailNameRe.MatchString(name) {
			return uniqueEmailString(f, col.MaxLength)
		}
		if imageNameRe.MatchString(name) {
			return uniqueImageString(f, col.MaxLength)
		}

		return uniqueGenericString(f, base, col.MaxLength)
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

const (
	emailSuffix = "@example.com"
	imagePrefix = "https://picsum.photos/seed/"
	imageSuffix = "/200/200"
	// genericTightThreshold is the MaxLength below which the generic
	// uniqueWrap stops trying to keep the base value (a phone number, name,
	// etc.) and emits a zero-padded counter instead. Below this width the
	// base would be truncated past recognition anyway.
	genericTightThreshold = 16
	// uniqueSuffixMinUUID is the minimum number of UUID characters reserved
	// after the "-" separator so the suffix always contributes to uniqueness.
	uniqueSuffixMinUUID = 8
)

// uniqueEmailString keeps the "<uuid>@example.com" shape when MaxLength allows,
// shortens the UUID local-part when only one or more local-part characters
// plus the full domain fit, and falls back to a base62 counter when MaxLength
// cannot hold even a single local-part character on top of the domain.
func uniqueEmailString(f *gofakeit.Faker, maxLen int) generator.Func {
	if maxLen <= 0 || maxLen >= len(emailSuffix)+36 {
		return func() any { return f.UUID() + emailSuffix }
	}
	if maxLen > len(emailSuffix) {
		local := maxLen - len(emailSuffix)
		return func() any {
			u := f.UUID()
			if len(u) > local {
				u = u[:local]
			}

			return u + emailSuffix
		}
	}

	return counterString(f, maxLen)
}

// uniqueImageString keeps the picsum URL shape when MaxLength allows, shortens
// the seed UUID when the URL skeleton still fits, and falls back to a base62
// counter for very tight widths.
func uniqueImageString(f *gofakeit.Faker, maxLen int) generator.Func {
	fixed := len(imagePrefix) + len(imageSuffix)
	if maxLen <= 0 || maxLen >= fixed+36 {
		return func() any {
			return imagePrefix + f.UUID() + imageSuffix
		}
	}
	if maxLen > fixed {
		seedLen := maxLen - fixed
		return func() any {
			u := f.UUID()
			if len(u) > seedLen {
				u = u[:seedLen]
			}

			return imagePrefix + u + imageSuffix
		}
	}

	return counterString(f, maxLen)
}

// uniqueGenericString stitches "<base>-<uuid>" while reserving room for a
// separator and at least uniqueSuffixMinUUID UUID characters, so the UUID
// portion always survives and keeps the value distinct. Trimming the base
// happens in rune units to avoid splitting a multi-byte UTF-8 character.
// When MaxLength is too tight to keep both base and a useful UUID suffix, the
// generator falls back to a base62 counter instead.
func uniqueGenericString(f *gofakeit.Faker, base generator.Func, maxLen int) generator.Func {
	if maxLen <= 0 {
		return func() any {
			v, ok := base().(string)
			if !ok {
				return f.UUID()
			}

			return v + "-" + f.UUID()
		}
	}
	if maxLen < genericTightThreshold {
		return counterString(f, maxLen)
	}

	return func() any {
		v, ok := base().(string)
		if !ok {
			v = f.UUID()
		}
		baseRoom := maxLen - 1 - uniqueSuffixMinUUID // reserve "-" + min UUID chars
		runes := []rune(v)
		if len(runes) > baseRoom {
			v = string(runes[:baseRoom])
		}
		uuidRoom := min(maxLen-utf8.RuneCountInString(v)-1, 36)
		u := f.UUID()
		if len(u) > uuidRoom {
			u = u[:uuidRoom]
		}

		return v + "-" + u
	}
}

// counterAlphabet is a 36-character case-fold-safe set: digits + uppercase
// letters only. MySQL's default collation (utf8mb4_0900_ai_ci) treats
// uppercase and lowercase as equal, so a mixed-case alphabet collides under
// UNIQUE constraints. Sticking to a single case gives a width-N column up
// to 36^N distinct values (46656 for varchar(3), ~60M for varchar(5));
// counterSpace additionally caps the modulus at uint64 max, so widths >= 13
// stop at 2^64-1 even though 36^N is larger.
const counterAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// counterString returns a generator that emits a base36-encoded counter
// exactly maxLen characters wide. The counter starts at a faker-seeded random
// offset so re-running seeder in append mode against the same table does not
// immediately collide with values inserted by a previous run. The counter
// wraps within min(36^maxLen, 2^64-1), so widths smaller than the row count
// eventually repeat.
func counterString(f *gofakeit.Faker, maxLen int) generator.Func {
	space := counterSpace(maxLen)
	counter := f.Uint64() % space

	return func() any {
		counter++

		return encodeBase36(counter%space, maxLen)
	}
}

// counterSpace returns 36^width, capped at uint64 max for widths that exceed
// what uint64 can represent (36^13 already exceeds 2^64).
func counterSpace(width int) uint64 {
	if width <= 0 {
		return 1
	}
	if width >= 13 {
		return ^uint64(0)
	}
	var n uint64 = 1
	for range width {
		n *= 36
	}

	return n
}

func encodeBase36(v uint64, width int) string {
	buf := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		buf[i] = counterAlphabet[v%36]
		v /= 36
	}

	return string(buf)
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
