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
	Func func(args []Arg, obj *rbxfile.Instance, sobj []int, sprop []string) (om []OutMap, err error)
}
type InPattern struct {
	Args []ArgType
	Func func(args []Arg, path string) (sfile []string, err error)
}
type InFilter struct {
	Args []ArgType
	Func func(args []Arg, im []InMap) (is []InSelection, err error)
}

type OutAction struct {
	Depth int
	Dir   []string
	Map   OutMap
}

// associates a selection with a file
type OutMap struct {
	File      FileDef
	Selection []OutSelection
}

// Selects items from a source object.
type OutSelection struct {
	// The source object.
	Object *rbxfile.Instance
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

func (fd FuncDef) CallOut(opt *Options, pair rulePair, obj *rbxfile.Instance) (om []OutMap, err error) {
	if pair.SyncType != SyncOut {
		err = errors.New("expected sync-out function pair")
		return
	}

	patternFn, ok := fd.OutPattern[pair.Pattern.Name]
	if !ok {
		err = errors.New("unknown pattern function")
		return
	}
	filterFn, ok := fd.OutFilter[pair.Filter.Name]
	if !ok {
		err = errors.New("unknown filter function")
		return
	}

	sobj, sprop, err := patternFn.Func(pair.Pattern.Args, obj)
	if err != nil {
		err = fmt.Errorf("pattern error: %s", err.Error())
		return
	}
	om, err = filterFn.Func(pair.Filter.Args, obj, sobj, sprop)
	if err != nil {
		err = fmt.Errorf("filter error: %s", err.Error())
	}
	return
}

func (fd FuncDef) CallIn(opt *Options, pair rulePair, path string) (im []InMap, is []InSelection, err error) {
	if pair.SyncType != SyncIn {
		err = errors.New("expected sync-in function pair")
		return
	}

	patternFn, ok := fd.InPattern[pair.Pattern.Name]
	if !ok {
		err = errors.New("unknown pattern function")
		return
	}
	filterFn, ok := fd.InFilter[pair.Filter.Name]
	if !ok {
		err = errors.New("unknown filter function")
		return
	}

	sfile, err := patternFn.Func(pair.Pattern.Args, path)
	if err != nil {
		err = fmt.Errorf("pattern error: %s", err.Error())
		return
	}

	im = make([]InMap, len(sfile))
	for i, name := range sfile {
		var format Format
		switch ext := filepath.Ext(name); ext {
		case ".rbxm":
			format = FormatRBXM{
				API: opt.API,
			}
		case ".rbxmx":
			format = FormatRBXMX{
				API: opt.API,
			}
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

	is, err = filterFn.Func(pair.Filter.Args, im)
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

func isValidFileName(name string, isDir bool) bool {
	if len(name) == 0 || len(name) > 255 ||
		name == "." || name == ".." {
		return false
	}
	for _, r := range name {
		switch {
		case 'A' <= r && r <= 'Z':
		case 'a' <= r && r <= 'z':
		case '0' <= r && r <= '9':
		case r == '.':
		case r == '_':
		case r == '-':
		default:
			return false
		}
	}
	return true
}

var DefaultRuleDefs = &FuncDef{
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
			Func: func(args []Arg, obj *rbxfile.Instance, sobj []int, sprop []string) (om []OutMap, err error) {
				name := string(args[0].(ArgString))
				var format Format
				switch ext := filepath.Ext(name); ext {
				case "rbxm":
					format = FormatRBXM{}
				case "rbxmx":
					format = FormatRBXMX{}
				case "json":
					format = FormatJSON{}
				case "xml":
					format = FormatXML{}
				default:
					return nil, errors.New("unsupported file extension")
				}

				sel := []OutSelection{{Object: obj, Children: sobj, Properties: sprop}}
				if !format.CanEncode(sel) {
					// ErrFormat{nobj, nprop, nval, Format}
					return nil, errors.New("selection not supported by format")
				}
				om = []OutMap{OutMap{File: FileDef{Name: name, IsDir: false}, Selection: sel}}
				return
			},
		},
		"Directory": {
			Args: []ArgType{},
			Func: func(args []Arg, obj *rbxfile.Instance, sobj []int, sprop []string) (om []OutMap, err error) {
				if len(sprop) > 0 {
					return nil, errors.New("properties not supported")
				}

				for _, n := range sobj {
					child := obj.Children[n]
					if !isValidFileName(child.Name(), true) {
						continue
					}
					om = append(om, OutMap{
						File:      FileDef{Name: child.Name(), IsDir: true},
						Selection: []OutSelection{{Object: obj, Children: []int{n}}},
					})
				}

				return
			},
		},
		"PropertyName": {
			Args: []ArgType{ArgTypeString},
			Func: func(args []Arg, obj *rbxfile.Instance, sobj []int, sprop []string) (om []OutMap, err error) {
				if len(sobj) > 0 {
					return nil, errors.New("objects not supported")
				}

				ext := strings.ToLower(string(args[0].(ArgString)))

				var format Format
				var typ rbxfile.Type
				switch ext {
				case "bin":
					format = FormatBin{}
					typ = rbxfile.TypeBinaryString
				case "lua":
					format = FormatLua{}
					typ = rbxfile.TypeProtectedString
				case "txt":
					format = FormatText{}
					typ = rbxfile.TypeString
				default:
					return nil, errors.New("unsupported format")
				}

				for _, name := range sprop {
					file := name + "." + ext
					if !isValidFileName(file, false) {
						continue
					}
					if obj.Properties[name].Type() != typ {
						continue
					}
					sel := []OutSelection{{Object: obj, Values: []string{name}}}
					if !format.CanEncode(sel) {
						continue
					}
					om = append(om, OutMap{
						File:      FileDef{Name: file, IsDir: false},
						Selection: sel,
					})
				}

				return
			},
		},
		"Ignore": {
			Args: []ArgType{},
			Func: func(args []Arg, obj *rbxfile.Instance, sobj []int, sprop []string) (om []OutMap, err error) {
				om = []OutMap{
					{
						File: FileDef{}, // empty filename: ignore selected items
						Selection: []OutSelection{
							{
								Object:     obj,
								Children:   sobj,
								Properties: sprop,
							},
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
	FuncType FuncType
	Name     string
	Args     []Arg
}

type rulePair struct {
	Depth    int
	SyncType SyncType
	Pattern  ruleFunc
	Filter   ruleFunc
}

type ruleParser struct {
	defs  *FuncDef
	r     io.Reader
	depth int
	err   error
	line  int
	funcs []rulePair
}

func (*ruleParser) trimSpace(s string) string {
	for i, r := range s {
		if !unicode.IsSpace(r) {
			return s[:i]
		}
	}
	return s
}

func (*ruleParser) ident(s string) string {
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

func (d *ruleParser) parseRules() (rp []rulePair, err error) {
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
	rp = d.funcs
	return
}

func (d *ruleParser) readLine(line string) {
	const ruleOpComment = "#"

	line = d.trimSpace(line)
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
	const ruleOpSep = ":"

	var syncType SyncType
	var patterns map[string][]ArgType
	var filters map[string][]ArgType

	typ := d.ident(rule)
	switch typ {
	case "out":
		syncType = SyncOut
		patterns = make(map[string][]ArgType, len(d.defs.OutPattern))
		for name, def := range d.defs.OutPattern {
			patterns[name] = def.Args
		}
		filters = make(map[string][]ArgType, len(d.defs.OutFilter))
		for name, def := range d.defs.OutFilter {
			filters[name] = def.Args
		}
	case "in":
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

	rule = d.trimSpace(rule)
	rule, rfp := d.readFunc(rule, patterns)
	rfp.FuncType = Pattern

	rule = d.trimSpace(rule)
	if strings.HasPrefix(rule, ruleOpSep) {
		rule = rule[len(ruleOpSep):]
	} else {
		d.err = fmt.Errorf("bad syntax: expected %q", ruleOpSep)
		return
	}

	rule = d.trimSpace(rule)
	rule, rff := d.readFunc(rule, filters)
	rff.FuncType = Filter

	d.funcs = append(d.funcs, rulePair{
		Depth:    d.depth,
		SyncType: syncType,
		Pattern:  rfp,
		Filter:   rff,
	})

	rule = d.trimSpace(rule)
	if len(rule) != 0 {
		d.err = errors.New("unexpected characters beyond filter")
		return
	}
}

func (d *ruleParser) readFunc(rule string, args map[string][]ArgType) (left string, rf ruleFunc) {
	const ruleOpArgOpen = "("
	const ruleOpArgClose = ")"
	const ruleOpArgSep = ","

	rf.Name = d.ident(rule)
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
