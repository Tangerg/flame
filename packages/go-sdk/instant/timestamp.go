package instant

import "time"

// Timestamp declares the standard time codec's string shape at a portable
// boundary. Domain owners still decide its precision, range and whether zero
// is permitted; it carries no independent clock or lifecycle.
type Timestamp struct{ time.Time }

func (Timestamp) JSONSchemaModel() any { return "" }
