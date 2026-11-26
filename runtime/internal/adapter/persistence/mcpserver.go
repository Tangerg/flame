package persistence

type MCPServerStore struct {
	db       *database
	receipts *idempotencyStore
}
