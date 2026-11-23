package runtimebinding

func requireIdentity(operation, actual, expected string) error {
	if actual == "" {
		return runtimeContractViolation("%s returned a result without an id", operation)
	}
	if expected != "" && actual != expected {
		return runtimeContractViolation("%s returned id %q for %q", operation, actual, expected)
	}
	return nil
}

// A repeated identity would silently collapse two Runtime rows into one CLI row,
// and no wire constraint can observe it.
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
