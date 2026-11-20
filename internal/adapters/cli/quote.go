package cli

import "strings"

// singleQuoteTriggers are the characters that POSIX shells or PowerShell
// expand or escape inside double quotes.
const singleQuoteTriggers = "\"$`\\!%^"

// shellQuote renders name as one shell word for the get hint, so the printed
// command can be pasted into a POSIX shell or PowerShell:
//   - bare when every byte is in [A-Za-z0-9._@+=,:/-] and name does not start
//     with '-';
//   - else double-quoted, unless name contains any of " $ ` \ ! % ^, in which
//     case it is POSIX single-quoted, with each embedded ' replaced by the
//     four bytes quote, backslash, quote, quote (close, escape, reopen).
//
// Unsupported: cmd.exe for names containing any of those trigger characters,
// and PowerShell for names containing both ' and a trigger character, since
// that escape is POSIX-only.
func shellQuote(name string) string {
	switch {
	case isBareWord(name):
		return name
	case strings.ContainsAny(name, singleQuoteTriggers):
		return "'" + strings.ReplaceAll(name, "'", `'\''`) + "'"
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
