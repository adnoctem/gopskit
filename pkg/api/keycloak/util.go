package keycloak

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func CreateDisplayName(s string) string {
	return cases.Title(language.English).String(s)
}
