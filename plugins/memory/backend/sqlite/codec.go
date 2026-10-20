package sqlite

type scanRow interface{ Scan(...any) error }

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
