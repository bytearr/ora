package match

import (
	"regexp"
	"strings"
	"unicode"
)

var bitnessRe = regexp.MustCompile(`(?i)\(?\b(64-bit|32-bit|64bit|32bit|x64|x86)\b\)?`)

// SplitWords inserts spaces at camelCase and letter/digit boundaries:
// "PixelPaintStudio" -> "Pixel Paint Studio", "Python312" -> "Python 312".
func SplitWords(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		if i > 0 {
			p := rs[i-1]
			switch {
			case unicode.IsLower(p) && unicode.IsUpper(r):
				b.WriteRune(' ')
			case unicode.IsUpper(p) && unicode.IsUpper(r) && i+1 < len(rs) && unicode.IsLower(rs[i+1]):
				b.WriteRune(' ')
			case unicode.IsLetter(p) && unicode.IsDigit(r), unicode.IsDigit(p) && unicode.IsLetter(r):
				b.WriteRune(' ')
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Normalize lowercases, strips trademark signs and bitness markers, splits
// words and turns punctuation into single spaces.
func Normalize(s string) string {
	// Every bitness marker has a 3, 4, 6 or 8; most names have none, and
	// the regexp is the bulk of a program search otherwise.
	if strings.ContainsAny(s, "3468") {
		s = bitnessRe.ReplaceAllString(s, " ")
	}
	s = SplitWords(s)
	s = strings.ToLower(s)
	var b strings.Builder
	space := true
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteRune(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

func Compact(normalized string) string {
	return strings.ReplaceAll(normalized, " ", "")
}

func Tokens(normalized string) []string {
	return strings.Fields(normalized)
}
