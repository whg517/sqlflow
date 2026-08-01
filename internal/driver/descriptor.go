package driver

// Descriptor is the serializable capability profile of a data source type.
//
// It is the single source of truth the UI uses to decide which editor, actions
// and result renderer to present. Adding a data source type must not require
// touching UI conditionals — the type declares what it can do, and the UI reads
// it. Server-side authorization is unaffected: a declared capability only means
// "this driver can do it", never "this caller may do it".
type Descriptor struct {
	Type      string    `json:"type"`
	QueryForm QueryForm `json:"query_form"`

	Query           bool `json:"query"`
	TicketExec      bool `json:"ticket_exec"`
	Metadata        bool `json:"metadata"`
	TablePermission bool `json:"table_permission"`
	FieldMasking    bool `json:"field_masking"`
	SQLParse        bool `json:"sql_parse"`
	Export          bool `json:"export"`

	// Explain reports whether the driver can produce a query plan. It is not a
	// Capability bit because it is derived from the optional QueryExplainer
	// interface rather than declared up front.
	Explain bool `json:"explain"`

	// Parameterized reports whether the driver binds query parameters instead
	// of interpolating them into the query string.
	Parameterized bool `json:"parameterized"`
}

// DescribeType builds the Descriptor for a registered data source type without
// opening a connection.
func DescribeType(typeName string) (*Descriptor, error) {
	d, err := NewDriver(typeName)
	if err != nil {
		return nil, err
	}
	return Describe(d), nil
}

// Describe builds the Descriptor for a driver instance.
func Describe(d Driver) *Descriptor {
	caps := d.Capabilities()
	_, explain := d.(QueryExplainer)
	_, parameterized := d.(ParameterizedQueryExecutor)

	return &Descriptor{
		Type:            d.Type(),
		QueryForm:       d.QueryForm(),
		Query:           caps.Has(CapQuery),
		TicketExec:      caps.Has(CapTicketExec),
		Metadata:        caps.Has(CapMetadata),
		TablePermission: caps.Has(CapTableLevelPermission),
		FieldMasking:    caps.Has(CapFieldMasking),
		SQLParse:        caps.Has(CapSQLParse),
		Export:          caps.Has(CapExport),
		Explain:         explain,
		Parameterized:   parameterized,
	}
}
