package terminal

import (
	"regexp"
	"strings"
)

// ansi matches the escape sequences of a terminal: CSI, OSC and the short ones.
var ansi = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][0-2AB]|\x1b[=>78cDEHMNOZ]`)

// PlainText turns raw terminal output into what a person would read: no
// escape sequences, and each line as it ends up after carriage returns.
func PlainText(raw []byte) string {
	s := ansi.ReplaceAllString(string(raw), "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if j := strings.LastIndex(l, "\r"); j >= 0 {
			l = l[j+1:]
		}
		lines[i] = strings.Map(func(r rune) rune {
			if r < 0x20 && r != '\t' || r == 0x7f {
				return -1
			}
			return r
		}, strings.TrimRight(l, " "))
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// Tail keeps the last n lines of text.
func Tail(text string, n int) string {
	if n <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
