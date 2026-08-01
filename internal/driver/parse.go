package driver

// ParseFor parses a query using the driver registered for typeName.
//
// Parsing needs no connection, so this builds a throwaway driver instance from
// the registry. Callers get the driver's own understanding of the query instead
// of switching on the type name themselves — adding a data source type must not
// require touching the query, export or review paths.
func ParseFor(typeName, query string) (*ParseResult, error) {
	d, err := NewDriver(typeName)
	if err != nil {
		return nil, err
	}
	return d.Parse(query)
}
