package testsupport

import "github.com/Tangerg/flame/runtime/internal/application/ownership"

// NewAdmissionGate supplies an uncontended external backend for tests of local
// Run and Session admission. Shared-directory ownership uses the real adapter.
func NewAdmissionGate() *ownership.Gate {
	gate, err := ownership.NewGate(admissionBackend{})
	if err != nil {
		panic(err)
	}
	return gate
}

type admissionBackend struct{}

func (admissionBackend) TrySession(string) (ownership.Lease, bool, error) {
	return admissionLease{}, true, nil
}

func (admissionBackend) TryWorkingTree(string, bool) (ownership.Lease, bool, error) {
	return admissionLease{}, true, nil
}

type admissionLease struct{}

func (admissionLease) Release() {}
