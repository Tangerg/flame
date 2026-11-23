package instant

import "time"

type Timestamp struct{ time.Time }

func (Timestamp) JSONSchemaModel() any { return "" }
