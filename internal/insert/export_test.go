package insert

import (
	"testing"

	"github.com/mickamy/seeder/internal/generator"
)

var (
	InsertTable     = insertTable
	IsSerialDefault = isSerialDefault
	PlanColumns     = planColumns
)

func (c colSpec) Gen() generator.Func { return c.gen }

// SetMySQLMaxPlaceholders lowers the BulkInsert placeholder cap for the
// duration of a test, restoring the previous value via t.Cleanup so chunking
// behavior is exercisable without inserting tens of thousands of rows.
func SetMySQLMaxPlaceholders(t *testing.T, n int) {
	t.Helper()
	old := mysqlMaxPlaceholders
	mysqlMaxPlaceholders = n
	t.Cleanup(func() { mysqlMaxPlaceholders = old })
}
