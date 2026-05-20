package insert

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// sqlOutputDriver writes INSERT (and optional TRUNCATE) statements to an
// io.Writer instead of executing them. dialect ("mysql" / "postgres" /
// "postgresql") selects literal escaping and identifier quoting. Inserted
// rows are retained in-memory so FK / polymorphic pools downstream can pick
// from them (mirrors what Driver.ColumnValues does against a real DB).
type sqlOutputDriver struct {
	w       io.Writer
	dialect string
	written map[string][]map[string]any
}

func newSQLOutputDriver(w io.Writer, dialect string) *sqlOutputDriver {
	return &sqlOutputDriver{
		w:       w,
		dialect: dialect,
		written: make(map[string][]map[string]any),
	}
}

func (d *sqlOutputDriver) Close(_ context.Context) error { return nil }

func (d *sqlOutputDriver) Truncate(_ context.Context, tables []string) error {
	if len(tables) == 0 {
		return nil
	}
	for _, t := range tables {
		switch d.dialect {
		case "mysql":
			if _, err := fmt.Fprintf(d.w, "TRUNCATE TABLE %s;\n", quoteMySQLIdent(t)); err != nil {
				return fmt.Errorf("write truncate: %w", err)
			}
		default:
			if _, err := fmt.Fprintf(d.w, "TRUNCATE %s RESTART IDENTITY CASCADE;\n", pgx.Identifier{t}.Sanitize()); err != nil {
				return fmt.Errorf("write truncate: %w", err)
			}
		}
	}

	return nil
}

func (d *sqlOutputDriver) BulkInsert(_ context.Context, table string, columns []string, rows [][]any) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	quoteIdent := func(s string) string { return pgx.Identifier{s}.Sanitize() }
	if d.dialect == "mysql" {
		quoteIdent = quoteMySQLIdent
	}

	quoted := make([]string, len(columns))
	for i, c := range columns {
		quoted[i] = quoteIdent(c)
	}
	header := fmt.Sprintf("INSERT INTO %s (%s) VALUES\n", quoteIdent(table), strings.Join(quoted, ", "))
	if _, err := d.w.Write([]byte(header)); err != nil {
		return 0, fmt.Errorf("write header: %w", err)
	}
	for i, row := range rows {
		vals := make([]string, len(row))
		for j, v := range row {
			vals[j] = sqlLiteral(v, d.dialect)
		}
		sep := ","
		if i == len(rows)-1 {
			sep = ";"
		}
		if _, err := fmt.Fprintf(d.w, "  (%s)%s\n", strings.Join(vals, ", "), sep); err != nil {
			return 0, fmt.Errorf("write row: %w", err)
		}
	}

	for _, row := range rows {
		m := make(map[string]any, len(columns))
		for i, c := range columns {
			m[c] = row[i]
		}
		d.written[table] = append(d.written[table], m)
	}

	return int64(len(rows)), nil
}

func (d *sqlOutputDriver) ColumnValues(_ context.Context, table string, columns []string) (map[string][]any, error) {
	out := make(map[string][]any, len(columns))
	for _, row := range d.written[table] {
		for _, c := range columns {
			out[c] = append(out[c], row[c])
		}
	}

	return out, nil
}

func sqlLiteral(v any, dialect string) string {
	if v == nil {
		return "NULL"
	}
	switch x := v.(type) {
	case string:
		return "'" + strings.ReplaceAll(x, "'", "''") + "'"
	case bool:
		if dialect == "mysql" {
			if x {
				return "1"
			}
			return "0"
		}
		if x {
			return "TRUE"
		}
		return "FALSE"
	case int:
		return strconv.Itoa(x)
	case int8:
		return strconv.Itoa(int(x))
	case int16:
		return strconv.Itoa(int(x))
	case int32:
		return strconv.Itoa(int(x))
	case int64:
		return strconv.FormatInt(x, 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float32:
		return strconvFloat(float64(x))
	case float64:
		return strconvFloat(x)
	case time.Time:
		return "'" + x.UTC().Format("2006-01-02 15:04:05") + "'"
	case []byte:
		if dialect == "mysql" {
			return "X'" + hex.EncodeToString(x) + "'"
		}
		// Postgres bytea hex escape: '\x...'
		return `'\x` + hex.EncodeToString(x) + `'`
	}

	return "'" + strings.ReplaceAll(fmt.Sprintf("%v", v), "'", "''") + "'"
}

func strconvFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", f), "0"), ".")
}
