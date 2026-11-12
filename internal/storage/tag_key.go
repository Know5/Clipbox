package storage

import (
	"database/sql/driver"
	"strings"

	"modernc.org/sqlite"
)

// SQLite's built-in lower()/NOCASE cover ASCII. Use the same Unicode lower
// casing as the Go tag aggregation, while preserving users' display labels.
var tagKeyRegistrationError = sqlite.RegisterDeterministicScalarFunction("clipbox_tag_key", 1,
	func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		if len(args) == 0 || args[0] == nil {
			return "", nil
		}
		value, _ := args[0].(string)
		return strings.ToLower(value), nil
	})
