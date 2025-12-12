package lib

import (
	"golang.org/x/crypto/bcrypt"
)

func MakePassword(value string) string {
	hashed, err := bcrypt.GenerateFromPassword([]byte(value), bcrypt.DefaultCost)

	if err != nil {
		return ""
	}
	return string(hashed)
}
