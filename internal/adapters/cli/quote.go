package cli

import "strings"

// singleQuoteTriggers are the characters that POSIX shells or PowerShell
// expand, escape or end a string at inside double quotes. U+201C U+201D U+201E
// are PowerShell's typographic double quotes.
const singleQuoteTriggers = "\"$`\\!%^\u201c\u201d\u201e"

// singleQuoteEscaper rewrites the characters that end a single-quoted string.
// PowerShell also ends one at its typographic single quotes U+2018 U+2019
// U+201A U+201B; each is closed out and double-quoted on its own, which POSIX
// shells join back into one word and PowerShell splits into separate
// arguments, so a pasted hint fails rather than runs anything.
var singleQuoteEscaper = strings.NewReplacer(
	"'", `'\''`,
	"\u2018", "'\"\u2018\"'",
	"\u2019", "'\"\u2019\"'",
	"\u201a", "'\"\u201a\"'",
	"\u201b", "'\"\u201b\"'",
)

// shellQuote renders name as one shell word for the get hint, so the printed
// command can be pasted into a POSIX shell or PowerShell:
//   - bare when every byte is in [A-Za-z0-9._@+=,:/-] and name does not start
//     with '-';
//   - else double-quoted, unless name contains any of " $ ` \ ! % ^ or a
//     typographic double quote (U+201C U+201D U+201E), in which case it is
//     POSIX single-quoted, with each embedded ' replaced by the four bytes
//     quote, backslash, quote, quote (close, escape, reopen) and each
//     typographic single quote (U+2018 U+2019 U+201A U+201B) replaced by
//     quote, double quote, the character, double quote, quote.
//
// Typographic single quotes are literal inside double quotes in both shells.
//
// Unsupported: cmd.exe for names containing any of those trigger characters,
// and PowerShell for names containing a trigger character together with ' or
// a typographic single quote, since those escapes are POSIX-only.
func shellQuote(name string) string {
	switch {
	case isBareWord(name):
		return name
	case strings.ContainsAny(name, singleQuoteTriggers):
		return "'" + singleQuoteEscaper.Replace(name) + "'"
	default:
		return `"` + name + `"`
	}
}

func isBareWord(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") {
		return false
	}
	for i := range len(name) {
		if !isBareByte(name[i]) {
			return false
		}
	}
	return true
}

func isBareByte(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		return true
	default:
		return strings.IndexByte("._@+=,:/-", c) >= 0
	}
}
