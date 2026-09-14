package toolresult

import "errors"

// ValidateReferences requires a canonical set so checkpoint identity includes
// every retained body exactly once.
func ValidateReferences(ids []ID) error {
	for index, id := range ids {
		if err := id.Validate(); err != nil {
			return err
		}
		if index > 0 && ids[index-1] >= id {
			return errors.New("tool result references are not strictly ordered")
		}
	}
	return nil
}
