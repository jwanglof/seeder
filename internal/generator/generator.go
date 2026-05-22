package generator

import (
	"strings"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/mickamy/seeder/internal/introspect"
)

type Func func() any

func FromKind(f *gofakeit.Faker, kind introspect.Kind, enumValues []string) Func {
	switch kind {
	case introspect.KindBool:
		return func() any { return f.Bool() }
	case introspect.KindInt:
		return func() any { return f.Number(1, 100000) }
	case introspect.KindFloat:
		return func() any { return f.Float64Range(0, 100000) }
	case introspect.KindString:
		return func() any { return f.Word() }
	case introspect.KindUUID:
		return func() any { return f.UUID() }
	case introspect.KindDate, introspect.KindTime, introspect.KindTimestamp:
		return func() any { return f.PastDate() }
	case introspect.KindJSON:
		return func() any { return jsonValue(f) }
	case introspect.KindEnum:
		if len(enumValues) == 0 {
			return func() any { return nil }
		}

		return func() any { return enumValues[f.IntRange(0, len(enumValues)-1)] }
	case introspect.KindBytes:
		return func() any { return randomBytes(f) }
	case introspect.KindUnknown:
		fallthrough
	default:
		return func() any { return f.Word() }
	}
}

// IntForColumn returns a generator that stays inside the signed range of
// dataType (tinyint / smallint / mediumint / int / bigint, on either Postgres
// or MySQL). Unknown integer types fall back to the same range FromKind uses.
// Unsigned types are not distinguished here because the signed bounds are
// always safe under both signed and unsigned declarations.
func IntForColumn(f *gofakeit.Faker, dataType string) Func {
	minV, maxV := intRangeFor(dataType)

	return func() any { return f.Number(minV, maxV) }
}

func intRangeFor(dataType string) (int, int) {
	switch strings.ToLower(strings.TrimSpace(dataType)) {
	case "tinyint":
		return 0, 100 // signed tinyint max 127, unsigned 255
	case "smallint":
		return 0, 10000 // signed smallint max 32767
	case "mediumint":
		return 0, 100000 // signed mediumint max 8388607
	default:
		return 1, 100000
	}
}

func randomBytes(f *gofakeit.Faker) []byte {
	n := f.IntRange(4, 32)
	b := make([]byte, n)
	for i := range b {
		b[i] = f.Uint8()
	}

	return b
}

func jsonValue(f *gofakeit.Faker) string {
	b, err := f.JSON(nil)
	if err != nil {
		return "{}"
	}

	return string(b)
}
