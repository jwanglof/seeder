package introspect

type Schema struct {
	Tables []Table
}

type Table struct {
	Name        string
	Columns     []Column
	PrimaryKey  []string
	ForeignKeys []ForeignKey
}

type Column struct {
	Name string
	// DataType is the raw type name as returned by the driver
	// (e.g., "int", "enum" for MySQL; "integer", "USER-DEFINED" for Postgres).
	DataType string
	// UDTName names the underlying user-defined type when relevant.
	// On MySQL it is empty (enum labels live in EnumValues).
	// On Postgres it matches the pg_type name when DataType == "USER-DEFINED".
	UDTName string
	// EnumValues holds the labels for an enum column; nil for non-enum columns.
	EnumValues []string
	Kind       Kind
	Nullable   bool
	HasDefault bool
	IsIdentity bool
}

type ForeignKey struct {
	Name              string
	Columns           []string
	ReferencedTable   string
	ReferencedColumns []string
}

// Kind is a driver-agnostic classification of a column's value space, used
// by the generator to pick a value strategy without caring about the raw
// type name reported by MySQL or Postgres.
type Kind int

const (
	KindUnknown Kind = iota
	KindBool
	KindInt
	KindFloat
	KindString
	KindUUID
	KindDate
	KindTime
	KindTimestamp
	KindJSON
	KindEnum
	KindBytes
)

func (k Kind) String() string {
	switch k {
	case KindBool:
		return "bool"
	case KindInt:
		return "int"
	case KindFloat:
		return "float"
	case KindString:
		return "string"
	case KindUUID:
		return "uuid"
	case KindDate:
		return "date"
	case KindTime:
		return "time"
	case KindTimestamp:
		return "timestamp"
	case KindJSON:
		return "json"
	case KindEnum:
		return "enum"
	case KindBytes:
		return "bytes"
	case KindUnknown:
		return "unknown"
	}

	return "unknown"
}
