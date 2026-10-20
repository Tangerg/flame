package identity

import "regexp"

const DeclarationIDPattern = `^[a-z][a-z0-9._-]{0,63}$`

var declarationID = regexp.MustCompile(DeclarationIDPattern)

func ValidDeclarationID(value string) bool { return declarationID.MatchString(value) }
