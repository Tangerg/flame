package tool

type Profile string

const (
	ProfileDefault      Profile = ""
	ProfileVerification Profile = "verification"
	ProfileMethodChange Profile = "method_change"
	ProfileMethodReview Profile = "method_review"
)

func (p Profile) Valid() bool {
	switch p {
	case ProfileDefault, ProfileVerification, ProfileMethodChange, ProfileMethodReview:
		return true
	default:
		return false
	}
}

func (p Profile) Method() bool {
	return p == ProfileMethodChange || p == ProfileMethodReview
}
