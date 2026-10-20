package runtimebinding

// requireIdentity owns the identity contract for one Runtime result.
//
// An operation that acts on an existing record names the ID it asked about, so
// the comparison carries the check. An operation that mints one names nothing,
// and is the only kind for which the presence of an identity is the whole
// contract: a result with no ID is accepted here and then fails much later,
// against whichever consumer first tries to name the record by it.
func requireIdentity(operation, actual, expected string) error {
	if actual == "" {
		return runtimeContractViolation("%s returned a result without an id", operation)
	}
	if expected != "" && actual != expected {
		return runtimeContractViolation("%s returned id %q for %q", operation, actual, expected)
	}
	return nil
}

// requireUniqueIdentities owns the same contract for every catalog the CLI
// reads from Runtime. A repeated identity would silently collapse two Runtime
// rows into one CLI row, and a missing one is unusable as the key every later
// operation names that row by; neither is observable to a wire constraint.
func requireUniqueIdentities[Value any](
	operation string,
	values []Value,
	identity func(Value) string,
) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := identity(value)
		if key == "" {
			return runtimeContractViolation("%s returned a row without an identity", operation)
		}
		if _, duplicate := seen[key]; duplicate {
			return runtimeContractViolation("%s repeats %q", operation, key)
		}
		seen[key] = struct{}{}
	}
	return nil
}
