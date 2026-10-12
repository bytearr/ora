package launch

import (
	"errors"
	"strings"
)

// ExecArgs expands a desktop entry Exec line (already free of the general
// \s \n \t \r \\ escapes) into argv. files fill %f/%u (first) and %F/%U
// (all); %i, %c and %k use icon, name and the desktop file path.
// Deprecated codes are dropped.
func ExecArgs(execLine string, files []string, name, icon, desktopPath string) ([]string, error) {
	words, err := splitExec(execLine)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, w := range words {
		switch w {
		case "%f", "%u":
			if len(files) > 0 {
				out = append(out, files[0])
			}
			continue
		case "%F", "%U":
			out = append(out, files...)
			continue
		case "%i":
			if icon != "" {
				out = append(out, "--icon", icon)
			}
			continue
		}
		out = append(out, expandInline(w, files, name, desktopPath))
	}
	if len(out) == 0 {
		return nil, errors.New("empty Exec line")
	}
	return out, nil
}

func expandInline(w string, files []string, name, desktopPath string) string {
	if !strings.Contains(w, "%") {
		return w
	}
	var b strings.Builder
	for i := 0; i < len(w); i++ {
		if w[i] != '%' || i+1 == len(w) {
			b.WriteByte(w[i])
			continue
		}
		i++
		switch w[i] {
		case '%':
			b.WriteByte('%')
		case 'f', 'u', 'F', 'U':
			if len(files) > 0 {
				b.WriteString(files[0])
			}
		case 'c':
			b.WriteString(name)
		case 'k':
			b.WriteString(desktopPath)
		}
	}
	return b.String()
}

// splitExec splits on blanks outside double quotes. Inside quotes a
// backslash escapes ", `, $ and \.
func splitExec(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	started, quoted := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quoted && c == '\\' && i+1 < len(s) && strings.IndexByte("\"`$\\", s[i+1]) >= 0:
			i++
			cur.WriteByte(s[i])
		case quoted && c == '"':
			quoted = false
		case quoted:
			cur.WriteByte(c)
		case c == '"':
			quoted, started = true, true
		case c == ' ' || c == '\t':
			if started {
				out = append(out, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteByte(c)
			started = true
		}
	}
	if quoted {
		return nil, errors.New("unterminated quote in Exec line")
	}
	if started {
		out = append(out, cur.String())
	}
	return out, nil
}

// FileURI is a file:// URI with the characters escaped that D-Bus tools
// treat specially in arguments.
func FileURI(path string) string {
	var b strings.Builder
	b.WriteString("file://")
	for i := 0; i < len(path); i++ {
		c := path[i]
		if c == '/' || c == '-' || c == '_' || c == '.' || c == '~' ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b.WriteByte(c)
			continue
		}
		const hex = "0123456789ABCDEF"
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}
