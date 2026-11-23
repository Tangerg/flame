package terminal

import "fmt"

type formChange uint8

const (
	formChangeKeep formChange = iota + 1
	formChangeSet
	formChangeClear
)

func (c formChange) Validate() error {
	switch c {
	case formChangeKeep, formChangeSet, formChangeClear:
		return nil
	default:
		return fmt.Errorf("form change %d is invalid", c)
	}
}

func (c formChange) SetsValue() bool   { return c == formChangeSet }
func (c formChange) ClearsValue() bool { return c == formChangeClear }
