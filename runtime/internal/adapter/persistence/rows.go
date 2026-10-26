package persistence

// scanRow is the row surface shared by *sql.Row (QueryRow) and *sql.Rows (Query
// iteration), so one scan helper covers both a single Get and a List loop. Used
// by the goal / interrupt / agent-memory decoders.
type scanRow interface {
	Scan(dest ...any) error
}
