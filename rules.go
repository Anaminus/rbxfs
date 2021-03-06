package rbxfs

import (
	"bufio"
	"encoding/json"
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
	// properties whose values are actually unresolved references
	References map[string]bool
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

type ErrSyncFunc struct {
	SyncType SyncType
	FuncType FuncType
	Name     string
	Err      error
}

func (err ErrSyncFunc) Error() string {
	return fmt.Sprintf("%s-%s function %q: %s", err.SyncType, err.FuncType, err.Name, err.Err.Error())
}

type ErrUnknownSyncFunc struct {
	SyncType SyncType
	FuncType FuncType
	Name     string
}

func (err ErrUnknownSyncFunc) Error() string {
	return fmt.Sprintf("unknown %s-%s function %q", err.SyncType, err.FuncType, err.Name)
}

type ErrSyncPair struct {
	Expected, Got SyncType
}

func (err ErrSyncPair) Error() string {
	return fmt.Sprintf("expected sync-%s function pair, got sync-%s", err.Expected, err.Got)
}

func (fd FuncDef) CallOut(opt *Options, pair rulePair, obj *rbxfile.Instance) (om []OutMap, err error) {
	if pair.SyncType != SyncOut {
		err = ErrSyncPair{Expected: SyncOut, Got: pair.SyncType}
		return
	}

	patternFn, ok := fd.OutPattern[pair.Pattern.Name]
	if !ok {
		err = ErrUnknownSyncFunc{SyncType: SyncOut, FuncType: Pattern, Name: pair.Pattern.Name}
		return
	}
	filterFn, ok := fd.OutFilter[pair.Filter.Name]
	if !ok {
		err = ErrUnknownSyncFunc{SyncType: SyncOut, FuncType: Filter, Name: pair.Filter.Name}
		return
	}

	sobj, sprop, err := patternFn.Func(opt, pair.Pattern.Args, obj)
	if err != nil {
		err = ErrSyncFunc{SyncType: SyncOut, FuncType: Pattern, Name: pair.Pattern.Name, Err: err}
		return
	}
	if len(sobj) == 0 && len(sprop) == 0 {
		return
	}

	om, err = filterFn.Func(opt, pair.Filter.Args, obj, sobj, sprop)
	if err != nil {
		err = ErrSyncFunc{SyncType: SyncOut, FuncType: Filter, Name: pair.Filter.Name, Err: err}
	}
	return
}

func (fd FuncDef) CallIn(opt *Options, cache SourceCache, pair rulePair, dirname, subdir string, refs map[string]*rbxfile.Instance) (is []InSelection, err error) {
	if pair.SyncType != SyncIn {
		err = ErrSyncPair{Expected: SyncIn, Got: pair.SyncType}
		return
	}

	patternFn, ok := fd.InPattern[pair.Pattern.Name]
	if !ok {
		err = ErrUnknownSyncFunc{SyncType: SyncIn, FuncType: Pattern, Name: pair.Pattern.Name}
		return
	}
	filterFn, ok := fd.InFilter[pair.Filter.Name]
	if !ok {
		err = ErrUnknownSyncFunc{SyncType: SyncIn, FuncType: Filter, Name: pair.Filter.Name}
		return
	}

	sfile, err := patternFn.Func(opt, pair.Pattern.Args, filepath.Join(dirname, subdir))
	if err != nil {
		err = ErrSyncFunc{SyncType: SyncIn, FuncType: Pattern, Name: pair.Pattern.Name, Err: err}
		return
	}
	if len(sfile) == 0 {
		return
	}

	errs := make(ErrsFile, 0, len(sfile))
	sm := make([]SourceMap, 0, len(sfile))
	for _, name := range sfile {
		relname := filepath.Join(subdir, name)
		scItem, ok := cache[relname]
		if !ok {
			r, err := os.Open(filepath.Join(opt.Repo, dirname, relname))
			if err != nil {
				errs = append(errs, &ErrFile{FileName: relname, Errors: []error{err}})
				continue
			}
			defer r.Close()
			stat, err := r.Stat()
			if err != nil {
				errs = append(errs, &ErrFile{FileName: relname, Errors: []error{err}})
				continue
			}

			scItem.IsDir = stat.IsDir()
			if scItem.IsDir {
				obj := &rbxfile.Instance{Properties: make(map[string]rbxfile.Value, 0)}
				if err := readAuxData(filepath.Join(opt.Repo, dirname, relname), obj); err != nil {
					// Ignore directory.
					continue
				}
				rbxfile.GetReference(obj, refs)
				obj.SetName(name)
				scItem.Source = &ItemSource{Children: []*rbxfile.Instance{obj}}
			} else {
				format := GetFormatFromExt(filepath.Ext(name))
				if format == nil {
					err := ErrSyncFunc{SyncType: SyncIn, FuncType: Pattern, Name: pair.Pattern.Name, Err: ErrUnsupportedFormat{Format: filepath.Ext(name)}}
					errs = append(errs, &ErrFile{FileName: relname, Errors: []error{err}})
					continue
				}
				format.SetAPI(opt.API)
				format.SetReferences(refs)
				var err error
				scItem.Source, err = format.Decode(r)
				if err != nil {
					errs = append(errs, &ErrFile{FileName: relname, Errors: []error{err}})
					continue
				}
			}

			cache[relname] = scItem
		}
		sm = append(sm, SourceMap{File: name, SourceCacheItem: scItem})
	}

	if len(errs) > 0 {
		return nil, errs
	}

	is, err = filterFn.Func(opt, pair.Filter.Args, sm)
	if err != nil {
		err = ErrSyncFunc{SyncType: SyncIn, FuncType: Filter, Name: pair.Filter.Name, Err: err}
	}
	return is, err
}

type auxData struct {
	ClassName string `json:"class_name"`
	Reference string `json:"reference"`
	IsService bool   `json:"is_service"`
}

const auxDataFileName = "data"

func writeAuxData(path string, obj *rbxfile.Instance) error {
	data := auxData{
		ClassName: obj.ClassName,
		Reference: obj.Reference,
		IsService: obj.IsService,
	}
	b, err := json.MarshalIndent(&data, "", "\t")
	if err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(path, auxDataFileName))
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(b)
	return err
}

func readAuxData(path string, obj *rbxfile.Instance) error {
	var data auxData
	b, err := ioutil.ReadFile(filepath.Join(path, auxDataFileName))
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, &data)
	if err != nil {
		return err
	}

	obj.ClassName = data.ClassName
	obj.Reference = data.Reference
	obj.IsService = data.IsService

	return nil
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
					return nil, ErrUnsupportedFormat{Format: filepath.Ext(name)}
				}

				sel := []OutSelection{{Object: obj, Children: sobj, Properties: sprop}}
				if !format.CanEncode(sel) {
					return nil, ErrFormatSelection{Format: format.Name()}
				}

				om = []OutMap{OutMap{File: FileDef{Name: name, IsDir: false}, Selection: sel}}
				return
			},
		},
		"Directory": {
			Args: []ArgType{},
			Func: func(opt *Options, args []Arg, obj *rbxfile.Instance, sobj []int, sprop []string) (om []OutMap, err error) {
				if len(sprop) > 0 {
					return nil, errors.New("property selections incompatible with filter")
				}

			loop:
				for _, n := range sobj {
					child := obj.Children[n]
					if !isValidFileName(child.Name(), true) {
						continue loop
					}
					for i, c := range obj.Children {
						if i == n {
							continue
						}
						if c.Name() == child.Name() {
							// Fail if child shares its name with any other
							// sibling.
							continue loop
						}
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
					return nil, errors.New("object selections incompatible with filter")
				}

				ext := strings.ToLower(string(args[0].(ArgString)))

				var format Format
				var typ rbxfile.Type
				switch ext {
				case "bin":
					format = &FormatBin{}
					typ = rbxfile.TypeBinaryString
				case "lua":
					format = &FormatLua{}
					typ = rbxfile.TypeProtectedString
				case "txt":
					format = &FormatText{}
					typ = rbxfile.TypeString
				default:
					return nil, ErrUnsupportedFormat{Format: ext}
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
				name := args[0].(ArgFileName)
				files, err := ioutil.ReadDir(filepath.Join(opt.Repo, path))
				if err != nil {
					return
				}
				for _, file := range files {
					if file.IsDir() {
						continue
					}
					if name.Match(file.Name()) {
						sfile = append(sfile, file.Name())
					}
				}

				return
			},
		},
		"Directory": {
			Args: []ArgType{ArgTypeClass, ArgTypeFileName},
			Func: func(opt *Options, args []Arg, path string) (sfile []string, err error) {
				class := args[0].(ArgClass)
				name := args[1].(ArgFileName)
				dir := filepath.Join(opt.Repo, path)
				files, err := ioutil.ReadDir(dir)
				if err != nil {
					return
				}
				for _, file := range files {
					if !file.IsDir() {
						continue
					}
					if !class.Name.Any {
						aux := rbxfile.NewInstance("", nil)
						if err := readAuxData(filepath.Join(dir, file.Name()), aux); err != nil {
							continue
						}
						if aux.ClassName == "" {
							continue
						}
						// Ignore directory if aux data is invalid or cannot
						// be read.

						api := opt.API
						if class.NoSub {
							api = nil
						}
						if !inherits(api, aux, class.Name.Literal) {
							continue
						}
					}

					if name.Match(file.Name()) {
						sfile = append(sfile, file.Name())
					}
				}

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
						return nil, errors.New("property and value sources incompatible with filter")
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
						return nil, errors.New("children and value sources incompatible with filter")
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
					return nil, errors.New("source must match exactly one file")
				}
				m := sm[0]
				if len(m.Source.Children) > 0 ||
					len(m.Source.Properties) > 0 ||
					len(m.Source.Values) != 1 {
					return nil, errors.New("source must contain only one value")
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
						return nil, errors.New("source must contain only one value")
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

// ErrParseRule is an error occurring when parsing a particular line of a rule
// file.
type ErrParseRule struct {
	Line int
	Err  error
}

func (err ErrParseRule) Error() string {
	return fmt.Sprintf("line %d: %s", err.Line, err.Err.Error())
}

// ErrsParseRule is an error containing a number of *ErrParseRule items.
type ErrsParseRule []*ErrParseRule

func (err ErrsParseRule) Error() string {
	return fmt.Sprintf("%d errors when parsing rules", len(err))
}

type ruleParser struct {
	defs  *FuncDef
	r     io.Reader
	depth int
	err   error // error per line
	line  int
	funcs []rulePair
	errs  ErrsParseRule // errors over all lines
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

func (d *ruleParser) parseRules() (rp []rulePair, err error) {
	s := bufio.NewScanner(d.r)
	s.Split(bufio.ScanLines)
	d.line = 1
	for s.Scan() {
		d.readLine(s.Text())
		if d.err != nil {
			d.errs = append(d.errs, &ErrParseRule{Line: d.line, Err: d.err})
			d.err = nil
		}
		d.line++
	}
	if s.Err() != nil {
		d.errs = append(d.errs, &ErrParseRule{Line: d.line, Err: s.Err()})
	}
	if len(d.errs) > 0 {
		err = d.errs
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
	errs := make(ErrsFile, 0, 2)
	for i, path := range []string{globalRulePath(), projectRulePath(opt.Repo)} {
		r, err := parseRuleFile(opt, i+1, path)
		if err != nil {
			switch i {
			case 0:
				path = "(global rules)"
			case 1:
				path = "(project rules)"
			}

			ef := &ErrFile{FileName: path}

			switch err := err.(type) {
			case ErrsParseRule:
				ef.Errors = make([]error, len(err))
				for i, e := range err {
					ef.Errors[i] = e
				}
			default:
				ef.Errors = []error{err}
			}

			errs = append(errs, ef)
			continue
		}
		rules = append(rules, r...)
	}

	err = errs
	return
}
