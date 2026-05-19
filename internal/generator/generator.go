package generator

import (
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
