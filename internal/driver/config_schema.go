package driver

// Field kinds. They name how a value is entered, not what it means.
const (
	FieldText     = "text"
	FieldPassword = "password"
	FieldNumber   = "number"
	FieldSelect   = "select"
	FieldSwitch   = "switch"
)

// ConfigOption is one choice in a FieldSelect.
type ConfigOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ConfigField describes one input a driver needs in order to connect.
//
// Name is the key the value travels under, and where it lands is decided by
// Storage rather than by the caller guessing: the columns every data source
// shares are columns, and everything else goes to extra_config, which is the
// rule ADR-0011 and CLAUDE.md already state for the server side. The form had
// no way to express that, so Elasticsearch's five settings were named fields in
// the UI long after the backend stopped having columns for them.
type ConfigField struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	Required bool   `json:"required"`

	// Placeholder and Help are what the form shows; the driver owns them
	// because the driver is what knows a MongoDB URI from a SQLite path.
	Placeholder string `json:"placeholder,omitempty"`
	Help        string `json:"help,omitempty"`

	// Default seeds a new form. It is a string for every kind, including
	// number and switch, because it travels as JSON and the form parses it.
	Default string `json:"default,omitempty"`

	Options []ConfigOption `json:"options,omitempty"`

	// Storage says where the value belongs: a shared column, or extra_config.
	Storage FieldStorage `json:"storage"`

	// Secret marks a credential. It is stored encrypted and never read back, so
	// the form leaves it blank on edit and treats empty as "unchanged".
	Secret bool `json:"secret,omitempty"`

	// ShowWhen hides the field unless another field currently holds one of
	// these values. Elasticsearch needs it — an API key input is meaningless
	// under basic auth — and expressing it here is what keeps the condition out
	// of the form's own branching.
	ShowWhen *FieldCondition `json:"show_when,omitempty"`
}

// FieldStorage says which half of the request payload a value belongs in.
type FieldStorage string

const (
	// StorageColumn is a top-level request field backed by its own column.
	//
	// Most are shared by every data source — host, port, username, database,
	// the encrypted password. Two are not: PostgreSQL's sslmode and schema_name
	// predate the rule against per-driver columns, and are declared honestly
	// here rather than described as something they are not. New driver settings
	// go to StorageExtra.
	StorageColumn FieldStorage = "column"
	// StorageExtra is the extra_config JSON the driver decodes itself.
	StorageExtra FieldStorage = "extra"
)

// FieldCondition makes one field's visibility depend on another's value.
type FieldCondition struct {
	Field  string   `json:"field"`
	Equals []string `json:"equals"`
}

// SQLHostSchema is the host/port/credentials/database shape the three
// connection-string drivers share.
//
// It is a helper a driver calls, not a default the platform applies: SQLite's
// answer is a single path and Elasticsearch's has no host at all, so there is
// nothing sensible to fall back to. PostgreSQL appends its own two fields to
// what this returns.
func SQLHostSchema(defaultPort, databaseLabel, databasePlaceholder string) []ConfigField {
	return []ConfigField{
		hostField(),
		portField(defaultPort),
		usernameField(true),
		passwordField(true),
		databaseField(databaseLabel, databasePlaceholder, false),
		maxOpenField(),
	}
}

func hostField() ConfigField {
	return ConfigField{
		Name: "host", Label: "主机", Kind: FieldText, Required: true,
		Placeholder: "IP 或域名", Storage: StorageColumn,
	}
}

func portField(def string) ConfigField {
	return ConfigField{
		Name: "port", Label: "端口", Kind: FieldNumber, Required: true,
		Placeholder: "1-65535", Default: def, Storage: StorageColumn,
	}
}

func usernameField(required bool) ConfigField {
	return ConfigField{
		Name: "username", Label: "用户名", Kind: FieldText, Required: required,
		Placeholder: "数据库用户名", Storage: StorageColumn,
	}
}

func passwordField(required bool) ConfigField {
	return ConfigField{
		Name: "password", Label: "密码", Kind: FieldPassword, Required: required,
		Placeholder: "数据库密码", Storage: StorageColumn, Secret: true,
	}
}

func databaseField(label, placeholder string, required bool) ConfigField {
	return ConfigField{
		Name: "database", Label: label, Kind: FieldText, Required: required,
		Placeholder: placeholder, Storage: StorageColumn,
	}
}

func maxOpenField() ConfigField {
	return ConfigField{
		Name: "max_open", Label: "最大连接数", Kind: FieldNumber,
		Default: "10", Storage: StorageColumn,
	}
}

// DatasourceTypeInfo is everything a client needs to render a form for one
// data source type, before any datasource of that type exists.
type DatasourceTypeInfo struct {
	Type      string        `json:"type"`
	QueryForm QueryForm     `json:"query_form"`
	Fields    []ConfigField `json:"fields"`

	// PlaceholderStyle is how a parameterised template writes its bind tokens.
	//
	// The template UI used to render "$1" when the type name was "postgresql"
	// and "?" otherwise — a driver fact the server already owned in
	// TemplateDialect but never put on the wire.
	PlaceholderStyle PlaceholderStyle `json:"placeholder_style"`
}

// PlaceholderStyle names a bind-token syntax without naming a driver.
type PlaceholderStyle string

const (
	// PlaceholderNumbered is $1, $2, $3.
	PlaceholderNumbered PlaceholderStyle = "numbered"
	// PlaceholderPositional is a bare ? per value.
	PlaceholderPositional PlaceholderStyle = "positional"
	// PlaceholderNone means values are inlined; there is nothing to bind to.
	PlaceholderNone PlaceholderStyle = "none"
)

// placeholderStyle derives the style from what the driver actually emits,
// rather than from a second declaration that could disagree with it.
func placeholderStyle(d Driver) PlaceholderStyle {
	binder, ok := d.(ParameterBinder)
	if !ok {
		return PlaceholderNone
	}
	switch binder.Placeholder(1) {
	case "?":
		return PlaceholderPositional
	case "$1":
		return PlaceholderNumbered
	default:
		return PlaceholderPositional
	}
}

// DatasourceTypes describes every registered type.
//
// This is what stops the UI from knowing the type names. The form used to hold
// the list twice — a hardcoded <SelectItem> per type and a default-port map —
// so a registered driver the UI had not been told about could not be selected
// at all.
func DatasourceTypes() ([]DatasourceTypeInfo, error) {
	names := SupportedTypes()
	out := make([]DatasourceTypeInfo, 0, len(names))
	for _, name := range names {
		d, err := NewDriver(name)
		if err != nil {
			return nil, err
		}
		out = append(out, DatasourceTypeInfo{
			Type:             d.Type(),
			QueryForm:        d.QueryForm(),
			Fields:           d.ConfigSchema(),
			PlaceholderStyle: placeholderStyle(d),
		})
	}
	return out, nil
}
