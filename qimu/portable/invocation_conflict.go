package portable

import "errors"

var ErrInvocationConflict = errors.New("invocation identity conflicts with the original request")
