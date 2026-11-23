package persistence

type scanRow interface {
	Scan(dest ...any) error
}
