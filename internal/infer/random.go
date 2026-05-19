package infer

import "github.com/brianvoe/gofakeit/v7"

func randomFrom(f *gofakeit.Faker, list []string) string {
	return list[f.Number(0, len(list)-1)]
}
