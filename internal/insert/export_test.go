package insert

import "github.com/mickamy/seeder/internal/generator"

var (
	InsertTable     = insertTable
	IsSerialDefault = isSerialDefault
	PlanColumns     = planColumns
)

func (c colSpec) Gen() generator.Func { return c.gen }
