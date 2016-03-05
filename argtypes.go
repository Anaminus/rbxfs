package rbxfs

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Arg interface {
	String() string
}
type ArgType func(s string) (Arg, int, error)

////////////////////////////////////////////////////////////////

type ArgString string

func (a ArgString) String() string {
	return string(a)
}

func ArgTypeString(s string) (a Arg, n int, err error) {
	v := []rune{}
loop:
	for n = 0; n < len(s); {
		switch r, size := utf8.DecodeRuneInString(s[n:]); r {
		case '\\':
			n += size
			if n >= len(s) {
				return nil, n, errors.New("reached end-of-line while parsing escape")
			}
			fallthrough
		default:
			n += size
			v = append(v, r)
		case ',', ')':
			break loop
		}
	}
	return ArgString(strings.TrimSpace(string(v))), n, nil
}

////////////////////////////////////////////////////////////////

type ArgName struct {
	Any     bool
	Literal string
}

func (a ArgName) String() string {
	if a.Any {
		return "*"
	}
	return a.Literal
}

func indexFunc(s string, f func(rune) bool, truth bool) int {
	start := 0
	for start < len(s) {
		wid := 1
		r := rune(s[start])
		if r >= utf8.RuneSelf {
			r, wid = utf8.DecodeRuneInString(s[start:])
		}
		if f(r) == truth {
			break
		}
		start += wid
	}
	return start
}

func ArgTypeName(s string) (a Arg, n int, err error) {
	arg := ArgName{}

	n = indexFunc(s, unicode.IsSpace, false)
	s = s[n:]
	if len(s) == 0 {
		return arg, n, nil
	}

	if s[0] == '*' {
		nn := indexFunc(s[1:], unicode.IsSpace, false)
		if s[1+nn] == ',' || s[1+nn] == ')' {
			arg.Any = true
			return arg, n + 1 + nn, nil
		}
	}

	v, nn, err := ArgTypeString(s)
	arg.Literal = string(v.(ArgString))
	return arg, n + nn, err
}

////////////////////////////////////////////////////////////////

type ArgClass struct {
	Name  ArgName
	NoSub bool
}

func (a ArgClass) String() string {
	s := a.Name.String()
	if a.NoSub {
		s = "@" + s
	}
	return s
}

func ArgTypeClass(s string) (a Arg, n int, err error) {
	arg := ArgClass{}

	n = indexFunc(s, unicode.IsSpace, false)
	s = s[n:]
	if len(s) == 0 {
		return arg, n, nil
	}

	if s[0] == '@' {
		arg.NoSub = true
		n++
		s = s[n:]
	}

	v, nn, err := ArgTypeName(s)
	arg.Name = v.(ArgName)
	return arg, n + nn, err
}

////////////////////////////////////////////////////////////////

type ArgFileName string

func (a ArgFileName) String() string {
	return string(a)
}

func (arg ArgFileName) Match(name string) bool {
	if arg == "*" {
		return true
	}
Pattern:
	for len(arg) > 0 {
		var star bool
		for len(arg) > 0 && arg[0] == '*' {
			arg = arg[1:]
			star = true
		}

		var i int
		ch := make([]byte, 0, len(arg))
		for i = 0; i < len(arg); i++ {
			if c := arg[i]; c == '\\' {
				i++
				if i >= len(arg) {
					return false // pattern error
				}
			} else if c == '*' {
				break
			}
			ch = append(ch, arg[i])
		}
		chunk := string(ch)

		if star && chunk == "" {
			return true
		}
		arg = arg[i:]
		if strings.HasPrefix(name, chunk) && (len(name) == len(chunk) || len(arg) > 0) {
			name = name[len(chunk):]
			continue
		}
		if star {
			for i := 0; i < len(name); i++ {
				if strings.HasPrefix(name[i+1:], chunk) {
					t := name[i+1+len(chunk):]
					if len(arg) == 0 && len(t) > 0 {
						continue
					}
					name = t
					continue Pattern
				}
			}
		}
		return false
	}
	return len(name) == 0
}

func ArgTypeFileName(s string) (a Arg, n int, err error) {
	str, n, err := ArgTypeString(s)
	return ArgFileName(str.(ArgString)), n, err
}
