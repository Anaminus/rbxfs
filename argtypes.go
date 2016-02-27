package rbxfs

import (
	"errors"
	"strings"
	"unicode/utf8"
)

type Arg interface{}
type ArgType func(s string) (Arg, int, error)

////////////////////////////////////////////////////////////////

type ArgString string

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
	return ArgString(v), n, nil
}

////////////////////////////////////////////////////////////////

type ArgName struct {
	Any     bool
	Literal string
}

func ArgTypeName(s string) (a Arg, n int, err error) {
	arg := ArgName{}
	if strings.HasPrefix(s, "*,") || strings.HasPrefix(s, "*)") {
		arg.Any = true
		return arg, 1, err
	}
	v, n, err := ArgTypeString(s)
	arg.Literal = string(v.(ArgString))
	return arg, n, err
}

////////////////////////////////////////////////////////////////

type ArgClass struct {
	Name  ArgName
	NoSub bool
}

func ArgTypeClass(s string) (a Arg, n int, err error) {
	arg := ArgClass{}
	if strings.HasPrefix(s, "@") {
		arg.NoSub = true
		s = s[1:]
		n++
	}
	v, nn, err := ArgTypeName(s)
	arg.Name = v.(ArgName)
	return arg, n + nn, err
}

////////////////////////////////////////////////////////////////

type ArgFileName string

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
