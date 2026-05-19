package introspect

type Schema struct {
	Tables []Table
	Enums  []Enum
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
	// (e.g., "integer", "USER-DEFINED" for Postgres; "int", "enum" for MySQL).
	DataType string
	// UDTName names the underlying user-defined type when relevant.
	// On Postgres it matches an Enum.Name when DataType == "USER-DEFINED".
	// On MySQL it is empty (enum labels are read directly into Enum).
	UDTName    string
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

type Enum struct {
	Name   string
	Values []string
}

// Kind is a driver-agnostic classification of a column's value space, used
// by the generator to pick a value strategy without caring about the raw
// type name reported by Postgres or MySQL.
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
