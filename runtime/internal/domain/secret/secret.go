package secret

const (
	Redacted = "[REDACTED]"
	Absent   = "<absent>"
)

func Presence(present bool) string {
	if present {
		return Redacted
	}
	return Absent
}
