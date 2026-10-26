package extension

// Refusal is a known business outcome, independent of transport failure.
type Refusal struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (r *Refusal) Error() string { return r.Message }
