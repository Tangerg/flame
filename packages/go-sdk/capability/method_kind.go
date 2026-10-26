package capability

import "fmt"

func KindFor(encoded []byte, name string) (MethodKind, error) {
	contract, err := Parse(encoded)
	if err != nil {
		return "", err
	}
	method, found := contract.Method(name)
	if !found {
		return "", fmt.Errorf("capability: unknown method %q", name)
	}
	return method.Kind(), nil
}
