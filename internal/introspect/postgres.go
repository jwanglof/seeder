package introspect

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"
)

type postgresDriver struct {
	conn *pgx.Conn
}

func newPostgresDriver(ctx context.Context, dataSourceName string) (*postgresDriver, error) {
	conn, err := pgx.Connect(ctx, dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	return &postgresDriver{conn: conn}, nil
}

func (d *postgresDriver) Close(ctx context.Context) error {
	if err := d.conn.Close(ctx); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	return nil
}

func (d *postgresDriver) Introspect(ctx context.Context) (Schema, error) {
	tables, err := d.fetchTablesWithColumns(ctx)
	if err != nil {
		return Schema{}, fmt.Errorf("fetch tables: %w", err)
	}
	if err := d.fetchPrimaryKeys(ctx, tables); err != nil {
		return Schema{}, fmt.Errorf("fetch primary keys: %w", err)
	}
	if err := d.fetchForeignKeys(ctx, tables); err != nil {
		return Schema{}, fmt.Errorf("fetch foreign keys: %w", err)
	}

	enums, err := d.fetchEnums(ctx)
	if err != nil {
		return Schema{}, fmt.Errorf("fetch enums: %w", err)
	}

	enumNames := make(map[string]struct{}, len(enums))
	for _, e := range enums {
		enumNames[e.Name] = struct{}{}
	}

	names := slices.Sorted(maps.Keys(tables))
	out := make([]Table, 0, len(names))
	for _, n := range names {
		t := tables[n]
		for i := range t.Columns {
			t.Columns[i].Kind = pgKind(t.Columns[i].DataType, t.Columns[i].UDTName, enumNames)
		}
		for _, fk := range t.ForeignKeys {
			if len(fk.Columns) > 1 {
				return Schema{}, fmt.Errorf(
					"table %s: composite FK %q (%d columns) is not supported in v0.1.0",
					t.Name, fk.Name, len(fk.Columns),
				)
			}
		}
		out = append(out, *t)
	}

	return Schema{Tables: out, Enums: enums}, nil
}

func pgKind(dataType, udtName string, enums map[string]struct{}) Kind {
	switch dataType {
	case "boolean":
		return KindBool
	case "smallint", "integer", "bigint":
		return KindInt
	case "real", "double precision", "numeric", "decimal":
		return KindFloat
	case "text", "character varying", "character", "name":
		return KindString
	case "uuid":
		return KindUUID
	case "date":
		return KindDate
	case "timestamp", "timestamp without time zone", "timestamp with time zone", "timestamptz":
		return KindTimestamp
	case "time", "time without time zone", "time with time zone", "timetz":
		return KindTime
	case "json", "jsonb":
		return KindJSON
	case "bytea":
		return KindBytes
	case "USER-DEFINED":
		if _, ok := enums[udtName]; ok {
			return KindEnum
		}

		return KindUnknown
	default:
		return KindUnknown
	}
}

const pgTablesQuery = `
SELECT
    c.table_name,
    c.column_name,
    c.data_type,
    c.udt_name,
    c.is_nullable,
    c.column_default IS NOT NULL AS has_default,
    c.is_identity
FROM information_schema.tables t
JOIN information_schema.columns c
  ON c.table_schema = t.table_schema
 AND c.table_name   = t.table_name
WHERE t.table_schema = 'public'
  AND t.table_type  = 'BASE TABLE'
ORDER BY c.table_name, c.ordinal_position
`

func (d *postgresDriver) fetchTablesWithColumns(ctx context.Context) (map[string]*Table, error) {
	rows, err := d.conn.Query(ctx, pgTablesQuery)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	tables := make(map[string]*Table)
	for rows.Next() {
		var (
			tname, cname, dataType, udtName string
			isNullable, isIdentity          string
			hasDefault                      bool
		)
		if err := rows.Scan(&tname, &cname, &dataType, &udtName, &isNullable, &hasDefault, &isIdentity); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		t, ok := tables[tname]
		if !ok {
			t = &Table{Name: tname}
			tables[tname] = t
		}
		t.Columns = append(t.Columns, Column{
			Name:       cname,
			DataType:   dataType,
			UDTName:    udtName,
			Nullable:   isNullable == "YES",
			HasDefault: hasDefault,
			IsIdentity: isIdentity == "YES",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return tables, nil
}

const pgPrimaryKeysQuery = `
SELECT kcu.table_name, kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name   = kcu.constraint_name
 AND tc.constraint_schema = kcu.constraint_schema
WHERE tc.constraint_type = 'PRIMARY KEY'
  AND tc.table_schema    = 'public'
ORDER BY kcu.table_name, kcu.ordinal_position
`

func (d *postgresDriver) fetchPrimaryKeys(ctx context.Context, tables map[string]*Table) error {
	rows, err := d.conn.Query(ctx, pgPrimaryKeysQuery)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tname, cname string
		if err := rows.Scan(&tname, &cname); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		if t, ok := tables[tname]; ok {
			t.PrimaryKey = append(t.PrimaryKey, cname)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows: %w", err)
	}

	return nil
}

// pg_constraint.conkey/confkey preserves composite FK column ordering
// (information_schema joins by constraint name and Cartesian-products it).
const pgForeignKeysQuery = `
SELECT
    con.conname,
    cl.relname  AS table_name,
    att.attname AS column_name,
    fcl.relname AS foreign_table_name,
    fatt.attname AS foreign_column_name
FROM pg_constraint con
JOIN pg_class      cl   ON cl.oid  = con.conrelid
JOIN pg_class      fcl  ON fcl.oid = con.confrelid
JOIN pg_namespace  ns   ON ns.oid  = cl.relnamespace
JOIN unnest(con.conkey, con.confkey) WITH ORDINALITY AS u(conkey, confkey, ord)
  ON TRUE
JOIN pg_attribute  att  ON att.attrelid  = con.conrelid  AND att.attnum  = u.conkey
JOIN pg_attribute  fatt ON fatt.attrelid = con.confrelid AND fatt.attnum = u.confkey
WHERE con.contype = 'f'
  AND ns.nspname  = 'public'
ORDER BY cl.relname, con.conname, u.ord
`

func (d *postgresDriver) fetchForeignKeys(ctx context.Context, tables map[string]*Table) error {
	rows, err := d.conn.Query(ctx, pgForeignKeysQuery)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	type fkKey struct{ table, name string }
	fks := make(map[fkKey]*ForeignKey)
	order := make([]fkKey, 0)
	for rows.Next() {
		var conName, tname, cname, fTable, fCol string
		if err := rows.Scan(&conName, &tname, &cname, &fTable, &fCol); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		key := fkKey{table: tname, name: conName}
		fk, ok := fks[key]
		if !ok {
			fk = &ForeignKey{Name: conName, ReferencedTable: fTable}
			fks[key] = fk
			order = append(order, key)
		}
		fk.Columns = append(fk.Columns, cname)
		fk.ReferencedColumns = append(fk.ReferencedColumns, fCol)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows: %w", err)
	}

	for _, k := range order {
		if t, ok := tables[k.table]; ok {
			t.ForeignKeys = append(t.ForeignKeys, *fks[k])
		}
	}

	return nil
}

const pgEnumsQuery = `
SELECT t.typname, e.enumlabel
FROM pg_type t
JOIN pg_enum e                 ON t.oid = e.enumtypid
JOIN pg_catalog.pg_namespace n ON n.oid = t.typnamespace
WHERE n.nspname = 'public'
ORDER BY t.typname, e.enumsortorder
`

func (d *postgresDriver) fetchEnums(ctx context.Context) ([]Enum, error) {
	rows, err := d.conn.Query(ctx, pgEnumsQuery)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	values := make(map[string][]string)
	for rows.Next() {
		var name, label string
		if err := rows.Scan(&name, &label); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		values[name] = append(values[name], label)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	names := slices.Sorted(maps.Keys(values))
	out := make([]Enum, 0, len(names))
	for _, n := range names {
		out = append(out, Enum{Name: n, Values: values[n]})
	}

	return out, nil
}
