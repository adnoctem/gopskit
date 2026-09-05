package helpers

import "unicode"

func OnlyLetters(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}

	return true
}

func NotEmptyString(s string) bool {
	return s != ""
}

func NotEmptyStrings(s, t string) bool {
	return s != "" && t != ""
}

func EmptyString(s string) bool {
	return s == ""
}

func EmptyStrings(s, t string) bool {
	return s == "" && t == ""
}
