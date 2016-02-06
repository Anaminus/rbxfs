package rbxfs

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/robloxapi/rbxfile"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type OutPattern struct {
	Args []ArgType
	Func func(args []Arg, obj *rbxfile.Instance) (sobj []int, sprop []string, err error)
}
type OutFilter struct {
	Args []ArgType
	Func func(args []Arg, sobj []int, sprop []string) (om []OutMap, err error)
}
type InPattern struct {
	Args []ArgType
	Func func(args []Arg, path string) (sfile []string, err error)
}
type InFilter struct {
	Args []ArgType
	Func func(args []Arg, im []InMap) (is []InSelection, err error)
}

// associates a selection with a file
type OutMap struct {
	File      FileDef
	Selection OutSelection
}

// Selects items from a source object.
type OutSelection struct {
	// A list of child objects selected from a source. Each value selects the
	// nth child.
	Children []int
	// A list of properties selected from a source. Each value is the name of
	// a property.
	Properties []string
	// A list of property values selected from a source. Each value is the
	// name of a property.
	Values []string
}

// Defines a file.
type FileDef struct {
	Name  string
	IsDir bool
}

type InMap struct {
	File   string
	Source ItemSource
}

// a source of items
type ItemSource struct {
	Children   []*rbxfile.Instance
	Properties map[string]rbxfile.Value
	// property values not mapped to any particular property
	Values []rbxfile.Value
}

type InSelection struct {
	File       string         // select items from source associated with named file
	Ignore     bool           // ignore associated file
	Children   []int          // add nth child to object
	Properties []string       // add named property to object
	Values     map[string]int // set named property to nth value
}

////////////////////////////////////////////////////////////////

type FuncDef struct {
	OutPattern map[string]OutPattern
	OutFilter  map[string]OutFilter
	InPattern  map[string]InPattern
	InFilter   map[string]InFilter
}

func (fd FuncDef) CallOut(pattern, filter ruleFunc, obj *rbxfile.Instance) (om []OutMap, err error) {
	if pattern.FuncType != Pattern {
		err = errors.New("pattern function is not a pattern")
		return
	}
	if pattern.FuncType != Filter {
		err = errors.New("filter function is not a filter")
		return
	}

	patternFn, ok := fd.OutPattern[pattern.Name]
	if !ok {
		err = errors.New("unknown pattern function")
		return
	}
	filterFn, ok := fd.OutFilter[filter.Name]
	if !ok {
		err = errors.New("unknown filter function")
		return
	}

	sobj, sprop, err := patternFn.Func(pattern.Args, obj)
	if err != nil {
		err = fmt.Errorf("pattern error: %s", err.Error())
		return
	}
	om, err = filterFn.Func(filter.Args, sobj, sprop)
	if err != nil {
		err = fmt.Errorf("filter error: %s", err.Error())
	}
	return
}
func (fd FuncDef) CallIn(pattern, filter ruleFunc, path string) (im []InMap, is []InSelection, err error) {
	if pattern.FuncType != Pattern {
		err = errors.New("pattern function is not a pattern")
		return
	}
	if pattern.FuncType != Filter {
		err = errors.New("filter function is not a filter")
		return
	}

	patternFn, ok := fd.InPattern[pattern.Name]
	if !ok {
		err = errors.New("unknown pattern function")
		return
	}
	filterFn, ok := fd.InFilter[filter.Name]
	if !ok {
		err = errors.New("unknown filter function")
		return
	}

	sfile, err := patternFn.Func(pattern.Args, path)
	if err != nil {
		err = fmt.Errorf("pattern error: %s", err.Error())
		return
	}

	im = make([]InMap, len(sfile))
	for i, name := range sfile {
		var format Format
		switch ext := filepath.Ext(name); ext {
		case ".rbxm":
			format = FormatRBXM{}
		case ".rbxmx":
			format = FormatRBXMX{}
		case ".json":
			format = FormatJSON{}
		case ".xml":
			format = FormatXML{}
		case ".bin":
			format = FormatBin{}
		case ".lua":
			format = FormatLua{}
		case ".txt":
			format = FormatText{}
		default:
			//error: pattern selected file with unsupported format
		}

		r, err := os.Open(name)
		if err != nil {
			//error?: cannot open file
		}
		im[i].Source, err = format.Decode(r)
		if err != nil {
			//error?: error decoding file
		}
		im[i].File = name
	}

	is, err = filterFn.Func(filter.Args, im)
	if err != nil {
		err = fmt.Errorf("filter error: %s", err.Error())
	}
	return
}

func inherits(obj *rbxfile.Instance, class string) bool {
	for obj != nil {
		if obj.ClassName == class {
			return true
		}
		obj = obj.Parent()
	}
	return false
}

var DefaultRuleFuncs = FuncDef{
	OutPattern: map[string]OutPattern{
		"Child": {
			Args: []ArgType{ArgTypeClass},
			Func: func(args []Arg, obj *rbxfile.Instance) (sobj []int, sprop []string, err error) {
				class := args[0].(ArgClass)
				if !class.Name.Any {
					if class.NoSub {
						if obj.ClassName != class.Name.Literal {
							return
						}
					} else {
						if !inherits(obj, class.Name.Literal) {
							return
						}
					}
				}
				sobj = make([]int, len(obj.Children))
				for i := range obj.Children {
					sobj[i] = i
				}
				return
			},
		},
		"Property": {
			Args: []ArgType{ArgTypeClass, ArgTypeName, ArgTypeName},
			Func: func(args []Arg, obj *rbxfile.Instance) (sobj []int, sprop []string, err error) {
				class := args[0].(ArgClass)
				prop := args[1].(ArgName)
				typ := args[2].(ArgName)
				if !class.Name.Any {
					if class.NoSub {
						if obj.ClassName != class.Name.Literal {
							return
						}
					} else {
						if !inherits(obj, class.Name.Literal) {
							return
						}
					}
				}
				for name, t := range obj.Properties {
					if prop.Any || name == prop.Literal {
						if typ.Any || strings.ToLower(t.Type().String()) == strings.ToLower(typ.Literal) {
							sprop = append(sprop, name)
						}
					}
				}
				return
			},
		},
	},
	OutFilter: map[string]OutFilter{
		"File": {
			Args: []ArgType{ArgTypeString},
			Func: func(args []Arg, sobj []int, sprop []string) (om []OutMap, err error) {
				name := string(args[0].(ArgString))
				switch ext := filepath.Ext(name); ext {
				case "rbxm":
				case "rbxmx":
				case "json":
				case "xml":
				default:
					return nil, errors.New("unsupported file extension")
				}

				// - get format from file extension
				// - check if format supports selected items
				return
			},
		},
		"Directory": {
			Args: []ArgType{},
			Func: func(args []Arg, sobj []int, sprop []string) (om []OutMap, err error) {
				if len(sprop) > 0 {
					return nil, errors.New("properties not supported")
				}
				// get file name from Name property of each object
				// if invalid, do not match object
				// !! name validation is done at file creation; can't be done here !!
				return
			},
		},
		"PropertyName": {
			Args: []ArgType{},
			Func: func(args []Arg, sobj []int, sprop []string) (om []OutMap, err error) {
				return
			},
		},
		"Ignore": {
			Args: []ArgType{},
			Func: func(args []Arg, sobj []int, sprop []string) (om []OutMap, err error) {
				om = []OutMap{
					OutMap{
						File: FileDef{}, // empty filename: ignore selected items
						Selection: OutSelection{
							Children:   sobj,
							Properties: sprop,
						},
					},
				}
				return
			},
		},
	},
	InPattern: map[string]InPattern{
		"File": {
			Args: []ArgType{ArgTypeFileName},
			Func: func(args []Arg, path string) (sfile []string, err error) {
				return
			},
		},
	},
	InFilter: map[string]InFilter{
		"Children": {
			Args: []ArgType{},
			Func: func(args []Arg, im []InMap) (is []InSelection, err error) {
				for _, m := range im {
					if len(m.Source.Properties) > 0 ||
						len(m.Source.Values) > 0 {
						// error: source not compatible with function
						return
					}
				}
				is = make([]InSelection, len(im))
				for i, m := range im {
					is[i] = InSelection{
						File:     m.File,
						Children: make([]int, len(m.Source.Children)),
					}
					for j := range m.Source.Children {
						is[i].Children[j] = j
					}
				}
				return
			},
		},
		"Properties": {
			Args: []ArgType{},
			Func: func(args []Arg, im []InMap) (is []InSelection, err error) {
				for _, m := range im {
					if len(m.Source.Children) > 0 ||
						len(m.Source.Values) > 0 {
						// error: source not compatible with function
						return
					}
				}
				is = make([]InSelection, len(im))
				for i, m := range im {
					is[i] = InSelection{
						File:       m.File,
						Properties: make([]string, len(m.Source.Properties)),
					}
					j := 0
					for name := range m.Source.Properties {
						is[i].Properties[j] = name
						j++
					}
				}
				return
			},
		},
		"Property": {
			Args: []ArgType{ArgTypeString},
			Func: func(args []Arg, im []InMap) (is []InSelection, err error) {
				name := string(args[0].(ArgString))
				if len(im) != 1 {
					// error: must match exactly one file
					return
				}
				m := im[0]
				if len(m.Source.Children) > 0 ||
					len(m.Source.Properties) > 0 ||
					len(m.Source.Values) != 1 {
					// error: source not compatible with function
					return
				}
				is = []InSelection{
					InSelection{
						File:   m.File,
						Values: map[string]int{name: 0},
					},
				}
				return
			},
		},
		"PropertyName": {
			Args: []ArgType{},
			Func: func(args []Arg, im []InMap) (is []InSelection, err error) {
				for _, m := range im {
					if len(m.Source.Children) > 0 ||
						len(m.Source.Properties) > 0 ||
						len(m.Source.Values) != 1 {
						// error: source not compatible with function
						return
					}
				}
				is = make([]InSelection, len(im))
				for i, m := range im {
					is[i] = InSelection{
						File:   m.File,
						Values: map[string]int{filepath.Base(m.File): 0},
					}
				}
				return
			},
		},
		"Ignore": {
			Args: []ArgType{},
			Func: func(args []Arg, im []InMap) (is []InSelection, err error) {
				is = make([]InSelection, len(im))
				for i, m := range im {
					is[i] = InSelection{
						File:   m.File,
						Ignore: true,
					}
				}
				return
			},
		},
	},
}

////////////////////////////////////////////////////////////////

type OutAction struct {
	Depth     int
	Selection OutMap
}

type InAction struct {
	Depth     int
	Selection InSelection
}

////////////////////////////////////////////////////////////////

const (
	ruleOpComment  = "#"
	ruleOpArgOpen  = "("
	ruleOpArgClose = ")"
	ruleOpArgSep   = ","
	ruleOpSep      = ":"

	ruleWordOut = "out"
	ruleWordIn  = "in"
)

type SyncType byte

const (
	SyncOut SyncType = iota
	SyncIn
)

type FuncType byte

const (
	Pattern FuncType = iota
	Filter
)

type ruleFunc struct {
	SyncType SyncType
	FuncType FuncType
	Name     string
	Args     []Arg
}

type ruleParser struct {
	defs  FuncDef
	r     io.Reader
	err   error
	line  int
	funcs []ruleFunc
}

func trimSpace(s string) string {
	for i, r := range s {
		if !unicode.IsSpace(r) {
			return s[:i]
		}
	}
	return s
}

func ident(s string) string {
	for i, r := range s {
		if ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') {
			continue
		}
		return s[:i]
	}
	return ""
}

type ErrRuleParser struct {
	Line int
	Err  error
}

func (err ErrRuleParser) Error() string {
	return fmt.Sprintf("line %d: %s", err.Line, err.Err.Error())
}

func (d *ruleParser) parseRules() (rf []ruleFunc, err error) {
	s := bufio.NewScanner(d.r)
	s.Split(bufio.ScanLines)
	d.line = 1
	for s.Scan() {
		d.readLine(s.Text())
		if d.err != nil {
			goto Error
		}
		d.line++
	}
	d.err = s.Err()
Error:
	if d.err != nil {
		err = ErrRuleParser{Line: d.line, Err: d.err}
		return
	}
	rf = d.funcs
	return
}

func (d *ruleParser) readLine(line string) {
	line = trimSpace(line)
	if len(line) == 0 {
		// empty
		return
	}
	if strings.HasPrefix(line, ruleOpComment) {
		// comment
		return
	}
	d.readRule(line)
}

func (d *ruleParser) readRule(rule string) {
	var syncType SyncType
	var patterns map[string][]ArgType
	var filters map[string][]ArgType

	typ := ident(rule)
	switch typ {
	case ruleWordOut:
		syncType = SyncOut
		patterns = make(map[string][]ArgType, len(d.defs.OutPattern))
		for name, def := range d.defs.OutPattern {
			patterns[name] = def.Args
		}
		filters = make(map[string][]ArgType, len(d.defs.OutFilter))
		for name, def := range d.defs.OutFilter {
			filters[name] = def.Args
		}
	case ruleWordIn:
		syncType = SyncIn
		patterns = make(map[string][]ArgType, len(d.defs.InPattern))
		for name, def := range d.defs.InPattern {
			patterns[name] = def.Args
		}
		filters = make(map[string][]ArgType, len(d.defs.InFilter))
		for name, def := range d.defs.InFilter {
			filters[name] = def.Args
		}
	default:
		d.err = fmt.Errorf("unknown rule type %q", typ)
		return
	}
	rule = rule[len(typ):]

	rule = trimSpace(rule)
	rule, rf := d.readFunc(rule, patterns)
	rf.SyncType = syncType
	rf.FuncType = Pattern
	d.funcs = append(d.funcs, rf)

	rule = trimSpace(rule)
	if strings.HasPrefix(rule, ruleOpSep) {
		rule = rule[len(ruleOpSep):]
	} else {
		d.err = fmt.Errorf("bad syntax: expected %q", ruleOpSep)
		return
	}

	rule = trimSpace(rule)
	rule, rf = d.readFunc(rule, filters)
	rf.SyncType = syncType
	rf.FuncType = Filter
	d.funcs = append(d.funcs, rf)

	rule = trimSpace(rule)
	if len(rule) != 0 {
		d.err = errors.New("unexpected characters beyond filter")
		return
	}
}

func (d *ruleParser) readFunc(rule string, args map[string][]ArgType) (left string, rf ruleFunc) {
	rf.Name = ident(rule)
	if len(rf.Name) == 0 {
		d.err = errors.New("empty function name")
		return
	}
	argts, ok := args[rf.Name]
	if !ok {
		d.err = fmt.Errorf("unknown function %q", rf.Name)
		return
	}

	rule = rule[len(rf.Name):]
	if !strings.HasPrefix(rule, ruleOpArgOpen) {
		d.err = fmt.Errorf("function %s: bad syntax: expected %q", rf.Name, ruleOpArgOpen)
		return
	}
	rule = rule[len(ruleOpArgOpen):]

	for i, argt := range argts {
		arg, n, err := argt(rule)
		if err != nil {
			d.err = fmt.Errorf("function %s: error parsing argument #%d: %s", rf.Name, i, err.Error())
			return
		}
		rule = rule[n:]
		rf.Args = append(rf.Args, arg)

		if i < len(argts)-1 {
			if strings.HasPrefix(rule, ruleOpArgClose) {
				d.err = fmt.Errorf("function %s: expected %d arguments, got %d", rf.Name, len(argts), i)
				return
			}
			if !strings.HasPrefix(rule, ruleOpArgSep) {
				d.err = fmt.Errorf("function %s: bad syntax: expected %q", rf.Name, ruleOpArgSep)
				return
			}
			rule = rule[len(ruleOpArgSep):]
		}
	}
	if !strings.HasPrefix(rule, ruleOpArgClose) {
		d.err = fmt.Errorf("function %s: bad syntax: expected %q", rf.Name, ruleOpArgClose)
		return
	}
	return rule[len(ruleOpArgClose):], rf
}

func ParseRules(r io.Reader) (rf []ruleFunc, err error) {
	p := &ruleParser{
		defs: DefaultRuleFuncs,
		r:    r,
	}
	return p.parseRules()
}
