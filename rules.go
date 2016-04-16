package rbxfs

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/robloxapi/rbxapi"
	"github.com/robloxapi/rbxfile"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type OutPattern struct {
	Args []ArgType
	Func func(opt *Options, args []Arg, obj *rbxfile.Instance) (sobj []int, sprop []string, err error)
}
type OutFilter struct {
	Args []ArgType
	Func func(opt *Options, args []Arg, obj *rbxfile.Instance, sobj []int, sprop []string) (om []OutMap, err error)
}
type InPattern struct {
	Args []ArgType
	Func func(opt *Options, args []Arg, path string) (sfile []string, err error)
}
type InFilter struct {
	Args []ArgType
	Func func(opt *Options, args []Arg, sm []SourceMap) (is []InSelection, err error)
}

// Defines a file.
type FileDef struct {
	Name  string
	IsDir bool
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
}

type InAction struct {
	Depth     int
	Dir       []string
	Selection []InSelection
}

type SourceMap struct {
	// The name of the file from which the source was derived.
	File string
	SourceCacheItem
}

type SourceCacheItem struct {
	IsDir  bool
	Source *ItemSource
}

// a source of items
type ItemSource struct {
	Children   []*rbxfile.Instance
	Properties map[string]rbxfile.Value
	// property values not mapped to any particular property
	Values []rbxfile.Value
}

// Maps a file name to an ItemSource. Name is relative to top directory of
// place.
type SourceCache map[string]SourceCacheItem

type InSelection struct {
	File       string         // select file name matching SourceMap.File
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

	sobj, sprop, err := patternFn.Func(opt, pair.Pattern.Args, obj)
	if err != nil {
		err = fmt.Errorf("pattern error: %s", err.Error())
		return
	}
	if len(sobj) == 0 && len(sprop) == 0 {
		return
	}

	om, err = filterFn.Func(opt, pair.Filter.Args, obj, sobj, sprop)
	if err != nil {
		err = fmt.Errorf("filter error: %s", err.Error())
	}
	return
}

// path: relative to opt.Repo
func (fd FuncDef) CallIn(opt *Options, cache SourceCache, pair rulePair, dir string) (is []InSelection, err error) {
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

	sfile, err := patternFn.Func(opt, pair.Pattern.Args, dir)
	if err != nil {
		err = fmt.Errorf("pattern error: %s", err.Error())
		return
	}

	sm := make([]SourceMap, len(sfile))
	for i, name := range sfile {
		relname := filepath.Join(dir, name)
		if _, ok := cache[relname]; ok {
			continue
		}

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

		r, err := os.Open(filepath.Join(opt.Repo, relname))
		if err != nil {
			//error?: cannot open file
		}
		defer r.Close()
		stat, err := r.Stat()
		if err != nil {
			//error?: cannot open file
		}

		scItem := SourceCacheItem{IsDir: stat.IsDir()}
		if scItem.IsDir {
			className, err := readClassNameFile(filepath.Join(opt.Repo, dir, name))
			if err != nil {
				//ERROR: ignore directory?
			}
			obj := rbxfile.NewInstance(className, nil)
			obj.SetName(name)
			scItem.Source = &ItemSource{Children: []*rbxfile.Instance{obj}}
		} else {
			scItem.Source, err = format.Decode(r)
			if err != nil {
				//error?: error decoding file
			}
		}

		cache[relname] = scItem
		sm[i] = SourceMap{File: name, SourceCacheItem: scItem}
	}

	is, err = filterFn.Func(opt, pair.Filter.Args, sm)
	if err != nil {
		err = fmt.Errorf("filter error: %s", err.Error())
	}
	return
}

const classNameFile = "ClassName"
const maxClassNameLength = 1 << 10

func readClassNameFile(dir string) (className string, err error) {
	name := filepath.Join(dir, classNameFile)
	stat, err := os.Stat(name)
	if err != nil {
		return "", err
	}
	if stat.Size() > maxClassNameLength {
		return "", errors.New("file too large")
	}
	b, err := ioutil.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func inherits(api *rbxapi.API, obj *rbxfile.Instance, className string) bool {
	if api == nil {
		return obj.ClassName == className
	}
	for class, ok := api.Classes[obj.ClassName]; ok; {
		if class.Name == className {
			return true
		}
		class, ok = api.Classes[class.Superclass]
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
			Func: func(opt *Options, args []Arg, obj *rbxfile.Instance) (sobj []int, sprop []string, err error) {
				class := args[0].(ArgClass)
				if class.Name.Any {
					sobj = make([]int, len(obj.Children))
					for i := range obj.Children {
						sobj[i] = i
					}
					return
				}

				api := opt.API
				if class.NoSub {
					api = nil
				}
				for i, child := range obj.Children {
					if inherits(api, child, class.Name.Literal) {
						sobj = append(sobj, i)
					}
				}

				return
			},
		},
		"Property": {
			Args: []ArgType{ArgTypeClass, ArgTypeName, ArgTypeName},
			Func: func(opt *Options, args []Arg, obj *rbxfile.Instance) (sobj []int, sprop []string, err error) {
				class := args[0].(ArgClass)
				prop := args[1].(ArgName)
				typ := args[2].(ArgName)

				if !class.Name.Any {
					api := opt.API
					if class.NoSub {
						api = nil
					}
					if !inherits(api, obj, class.Name.Literal) {
						return
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
			Func: func(opt *Options, args []Arg, obj *rbxfile.Instance, sobj []int, sprop []string) (om []OutMap, err error) {
				name := string(args[0].(ArgString))

				format := GetFormatFromExt(filepath.Ext(name))
				if format == nil {
					return nil, errors.New("unsupported file extension for " + name)
				}

				sel := []OutSelection{{Object: obj, Children: sobj, Properties: sprop}}
				if !format.CanEncode(sel) {
					// ErrFormat{nobj, nprop, nval, Format}
					// return
					return nil, errors.New("selection not supported by format")
				}

				om = []OutMap{OutMap{File: FileDef{Name: name, IsDir: false}, Selection: sel}}
				return
			},
		},
		"Directory": {
			Args: []ArgType{},
			Func: func(opt *Options, args []Arg, obj *rbxfile.Instance, sobj []int, sprop []string) (om []OutMap, err error) {
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
			Func: func(opt *Options, args []Arg, obj *rbxfile.Instance, sobj []int, sprop []string) (om []OutMap, err error) {
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
					_ = typ
					// if obj.Properties[name].Type() != typ {
					// 	continue
					// }
					sel := []OutSelection{{Object: obj, Properties: []string{name}}}
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
			Func: func(opt *Options, args []Arg, obj *rbxfile.Instance, sobj []int, sprop []string) (om []OutMap, err error) {
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
			Func: func(opt *Options, args []Arg, path string) (sfile []string, err error) {
				return
			},
		},
	},
	InFilter: map[string]InFilter{
		"Children": {
			Args: []ArgType{},
			Func: func(opt *Options, args []Arg, sm []SourceMap) (is []InSelection, err error) {
				for _, m := range sm {
					if len(m.Source.Properties) > 0 ||
						len(m.Source.Values) > 0 {
						// error: source not compatible with function
						return
					}
				}
				is = make([]InSelection, len(sm))
				for i, m := range sm {
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
			Func: func(opt *Options, args []Arg, sm []SourceMap) (is []InSelection, err error) {
				for _, m := range sm {
					if len(m.Source.Children) > 0 ||
						len(m.Source.Values) > 0 {
						// error: source not compatible with function
						return
					}
				}
				is = make([]InSelection, len(sm))
				for i, m := range sm {
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
			Func: func(opt *Options, args []Arg, sm []SourceMap) (is []InSelection, err error) {
				name := string(args[0].(ArgString))
				if len(sm) != 1 {
					// error: must match exactly one file
					return
				}
				m := sm[0]
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
			Func: func(opt *Options, args []Arg, sm []SourceMap) (is []InSelection, err error) {
				for _, m := range sm {
					if len(m.Source.Children) > 0 ||
						len(m.Source.Properties) > 0 ||
						len(m.Source.Values) != 1 {
						// error: source not compatible with function
						return
					}
				}
				is = make([]InSelection, len(sm))
				for i, m := range sm {
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
			Func: func(opt *Options, args []Arg, sm []SourceMap) (is []InSelection, err error) {
				is = make([]InSelection, len(sm))
				for i, m := range sm {
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

func (s SyncType) String() string {
	switch s {
	case SyncIn:
		return "in"
	case SyncOut:
		return "out"
	}
	return ""
}

const (
	SyncOut SyncType = iota
	SyncIn
)

type FuncType byte

func (f FuncType) String() string {
	switch f {
	case Pattern:
		return "pattern"
	case Filter:
		return "filter"
	}
	return ""
}

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

func (r rulePair) String() string {
	args := func(args []Arg) string {
		var s []string
		for _, arg := range args {
			s = append(s, arg.String())
		}
		return strings.Join(s, ", ")
	}
	return fmt.Sprintf("%d: %s %s(%s) : %s(%s)",
		r.Depth,
		r.SyncType,
		r.Pattern.Name,
		args(r.Pattern.Args),
		r.Filter.Name,
		args(r.Filter.Args),
	)
}

type ruleParser struct {
	defs  *FuncDef
	r     io.Reader
	depth int
	err   error
	line  int
	funcs []rulePair
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
	if s.Err() != nil {
		d.err = s.Err()
	}
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

	line = strings.TrimLeftFunc(line, unicode.IsSpace)
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

	rule = strings.TrimLeftFunc(rule, unicode.IsSpace)
	rule, rfp := d.readFunc(rule, patterns)
	if d.err != nil {
		return
	}
	rfp.FuncType = Pattern

	rule = strings.TrimLeftFunc(rule, unicode.IsSpace)
	if strings.HasPrefix(rule, ruleOpSep) {
		rule = rule[len(ruleOpSep):]
	} else {
		d.err = fmt.Errorf("bad syntax: expected %q", ruleOpSep)
		return
	}

	rule = strings.TrimLeftFunc(rule, unicode.IsSpace)
	rule, rff := d.readFunc(rule, filters)
	if d.err != nil {
		return
	}
	rff.FuncType = Filter

	d.funcs = append(d.funcs, rulePair{
		Depth:    d.depth,
		SyncType: syncType,
		Pattern:  rfp,
		Filter:   rff,
	})

	rule = strings.TrimLeftFunc(rule, unicode.IsSpace)
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
				d.err = fmt.Errorf("function %s: expected %d arguments, got %d", rf.Name, len(argts), i+1)
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

func parseRuleFile(opt *Options, depth int, path string) ([]rulePair, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	p := &ruleParser{
		defs:  opt.RuleDefs,
		r:     f,
		depth: depth,
	}
	if p.defs == nil {
		p.defs = DefaultRuleDefs
	}
	return p.parseRules()
}

func filterRuleType(rules []rulePair, typ SyncType) (out []rulePair) {
	for _, rule := range rules {
		if rule.SyncType == typ {
			out = append(out, rule)
		}
	}
	return
}

func getStdRules(opt *Options) (rules []rulePair, err error) {
	r, err := parseRuleFile(opt, 1, globalRulePath())
	if err != nil {
		//ERROR:
		fmt.Println("global rules:", err)
	} else {
		rules = append(rules, r...)
	}

	r, err = parseRuleFile(opt, 2, projectRulePath(opt.Repo))
	if err != nil {
		//ERROR:
		fmt.Println("project rules:", err)
	} else {
		rules = append(rules, r...)
	}

	rules = filterRuleType(rules, SyncOut)

	return
}
