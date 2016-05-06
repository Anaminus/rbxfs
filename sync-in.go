package rbxfs

import (
	"fmt"
	"github.com/robloxapi/rbxapi"
	"github.com/robloxapi/rbxapi/dump"
	"github.com/robloxapi/rbxfile"
	"github.com/robloxapi/rbxfile/bin"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ErrReadDir struct {
	Dir string
	Err error
}

func (err ErrReadDir) Error() string {
	return fmt.Sprintf("error reading dir %q: %s", err.Dir, err.Err.Error())
}

func syncInReadDir(opt *Options, cache SourceCache, dirname string, subdir []string, rules []rulePair, refs map[string]*rbxfile.Instance) (actions []InAction, err error) {
	defs := opt.RuleDefs
	if defs == nil {
		defs = DefaultRuleDefs
	}

	children := map[string]bool{}
	jdir := filepath.Join(subdir...)
	for _, pair := range rules {
		is, err := defs.CallIn(opt, cache, pair, dirname, jdir, refs)
		if err != nil {
			return nil, &ErrReadDir{Dir: jdir, Err: err}
		}
		for _, s := range is {
			// Scan for directories.
			if !s.Ignore && len(s.Children) == 1 {
				if source, ok := cache[filepath.Join(jdir, s.File)]; ok && source.IsDir {
					children[s.File] = true
				}
			}
			actions = append(actions, InAction{
				Depth:     pair.Depth,
				Dir:       subdir,
				Selection: []InSelection{s},
			})
		}
	}

	sorted := make([]string, len(children))
	{
		i := 0
		for name := range children {
			sorted[i] = name
			i++
		}
	}
	sort.Strings(sorted)

	for _, name := range sorted {
		sub := make([]string, len(subdir)+1)
		copy(sub, subdir)
		sub[len(sub)-1] = name
		a, err := syncInReadDir(opt, cache, dirname, sub, rules, refs)
		if err != nil {
			if err, ok := err.(*ErrReadDir); ok {
				return nil, err
			}
			return nil, &ErrReadDir{Dir: jdir, Err: err}
		}
		actions = append(actions, a...)
	}

	return
}

type OrderedInAction struct {
	Priority int
	Action   InAction
}

type SortInActionsByDepth []OrderedInAction

func (s SortInActionsByDepth) Len() int {
	return len(s)
}
func (s SortInActionsByDepth) Less(i, j int) bool {
	if s[i].Action.Depth == s[j].Action.Depth {
		return s[i].Priority < s[j].Priority
	}
	return s[i].Action.Depth < s[j].Action.Depth
}
func (s SortInActionsByDepth) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

type SortInActionsByDir []InAction

func (s SortInActionsByDir) Len() int {
	return len(s)
}
func (s SortInActionsByDir) Less(i, j int) bool {
	return filepath.Join(s[i].Dir...) < filepath.Join(s[j].Dir...)
}
func (s SortInActionsByDir) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

type SortInSelections []InSelection

func (s SortInSelections) Len() int {
	return len(s)
}
func (s SortInSelections) Less(i, j int) bool {
	if s[i].File == s[j].File {
		return !s[i].Ignore && s[j].Ignore
	}
	return s[i].File < s[j].File
}
func (s SortInSelections) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func syncInAnalyzeActions(actions []InAction) []InAction {
	// Conflicting Action pass: Resolve multiple actions selecting the same
	// item. Also separate actions into individual selections.
	{
		type childItem struct {
			path  string
			child int
		}
		type propItem struct {
			dir  string
			prop string
		}

		children := map[childItem]OrderedInAction{}
		properties := map[propItem]OrderedInAction{}
		for i, action := range actions {
			dir := filepath.Join(action.Dir...)
			for _, selection := range action.Selection {
				path := filepath.Join(dir, selection.File)
				for _, child := range selection.Children {
					// Objects may conflict only if they are from the same
					// source.
					//
					// TODO: This will probably change when reference tracking
					// is introduced.
					item := childItem{path, child}
					// Latter actions override former actions.
					if a, ok := children[item]; ok && a.Action.Depth > action.Depth {
						// Do not override if existing action is deeper.
						continue
					}
					children[item] = OrderedInAction{
						Priority: i,
						Action: InAction{
							Depth: action.Depth,
							Dir:   action.Dir,
							Selection: []InSelection{InSelection{
								File:     selection.File,
								Ignore:   selection.Ignore,
								Children: []int{child},
							}},
						},
					}
				}
				for _, prop := range selection.Properties {
					item := propItem{dir, prop}
					if a, ok := properties[item]; ok && a.Action.Depth > action.Depth {
						continue
					}
					properties[item] = OrderedInAction{
						Priority: i,
						Action: InAction{
							Depth: action.Depth,
							Dir:   action.Dir,
							Selection: []InSelection{InSelection{
								File:       selection.File,
								Ignore:     selection.Ignore,
								Properties: []string{prop},
							}},
						},
					}
				}
				for prop, value := range selection.Values {
					// Properties conflict per directory, rather than per
					// source.
					item := propItem{dir, prop}
					if a, ok := properties[item]; ok && a.Action.Depth > action.Depth {
						continue
					}
					properties[item] = OrderedInAction{
						Priority: i,
						Action: InAction{
							Depth: action.Depth,
							Dir:   action.Dir,
							Selection: []InSelection{InSelection{
								File:   selection.File,
								Ignore: selection.Ignore,
								Values: map[string]int{prop: value},
							}},
						},
					}
				}
			}
		}

		sorted := make([]OrderedInAction, 0, len(children)+len(properties))
		for _, action := range children {
			sorted = append(sorted, action)
		}
		for _, action := range properties {
			sorted = append(sorted, action)
		}
		sort.Sort(SortInActionsByDepth(sorted))

		actions = make([]InAction, len(sorted))
		for i, action := range sorted {
			actions[i] = action.Action
		}
	}

	// Merge pass 1: Combine selections of actions that apply to the same
	// directory.
	{
		combine := map[string]*InAction{}
		for i := 0; i < len(actions); i++ {
			action := actions[i]
			dir := filepath.Join(action.Dir...)
			if c, ok := combine[dir]; ok {
				c.Selection = append(c.Selection, action.Selection...)
			} else {
				combine[dir] = &action
			}
		}

		out := make([]InAction, 0, len(actions))
		for _, action := range combine {
			out = append(out, *action)
		}
		actions = out
	}

	// Merge pass 2: Combine selections within each action that apply to the
	// same source.
	{
		type selItem struct {
			file   string
			ignore bool
		}

		for i, action := range actions {
			combine := map[selItem]*InSelection{}
			for i := 0; i < len(action.Selection); i++ {
				selection := action.Selection[i]
				item := selItem{selection.File, selection.Ignore}
				if c, ok := combine[item]; ok {
					c.Children = append(c.Children, selection.Children...)
					c.Properties = append(c.Properties, selection.Properties...)
					for key, value := range selection.Values {
						c.Values[key] = value
					}
				} else {
					combine[item] = &selection
				}
			}

			action.Selection = make([]InSelection, 0, len(combine))
			for _, sel := range combine {
				action.Selection = append(action.Selection, *sel)
			}
			actions[i] = action
		}
	}

	// Sort pass: Sort actions, selections, and items.
	{
		for i, action := range actions {
			for i, sel := range action.Selection {
				sort.Ints(sel.Children)
				sort.Strings(sel.Properties)
				action.Selection[i] = sel
			}
			sort.Sort(SortInSelections(action.Selection))
			actions[i] = action
		}
		sort.Sort(SortInActionsByDir(actions))
	}
	return actions
}

func syncInVerifyActions(opt *Options, dir, place string, refs map[string]*rbxfile.Instance, cache SourceCache, actions []InAction) error {
	fmt.Printf("sync-in `%s` -> `%s`\n", filepath.Join(opt.Repo, dir), filepath.Join(opt.Repo, place))
	for i, action := range actions {
		var sel []string
		for _, s := range action.Selection {
			sel = append(sel, fmt.Sprintf("{file: %s; I: %t; C: %v; P: %v; V: %v}",
				s.File, s.Ignore, s.Children, s.Properties, s.Values,
			))
		}
		fmt.Printf("\t%4d %d; %-24s; sel(%02d): {%s}\n", i, action.Depth, "`"+filepath.Join(action.Dir...)+"`", len(action.Selection), strings.Join(sel, "; "))
	}
	return nil
}

func syncInApplyActions(opt *Options, dir, place string, refs map[string]*rbxfile.Instance, cache SourceCache, actions []InAction) error {
	datamodel := rbxfile.NewInstance("DataModel", nil)
	dirMap := map[string]*rbxfile.Instance{"": datamodel}
	for _, action := range actions {
		subdir := filepath.Join(action.Dir...)
		for _, selection := range action.Selection {
			if selection.Ignore {
				continue
			}

			source := cache[filepath.Join(subdir, selection.File)]
			if source.IsDir {
				dirMap[filepath.Join(subdir, selection.File)] = source.Source.Children[selection.Children[0]]
			}

			parent := dirMap[subdir]
			for _, child := range selection.Children {
				source.Source.Children[child].SetParent(parent)
			}
			for _, prop := range selection.Properties {
				if source.Source.References[prop] {
					if rbxfile.ResolveReference(refs, rbxfile.PropRef{
						Instance:  parent,
						Property:  prop,
						Reference: string(source.Source.Properties[prop].(rbxfile.ValueString)),
					}) {
						continue
					}
				} else {
					parent.Properties[prop] = source.Source.Properties[prop]
				}
			}
			for prop, value := range selection.Values {
				parent.Properties[prop] = source.Source.Values[value]
			}
		}
	}

	// Correct services based on predefined list.
	// TODO: make this better (extract info from exe?)
	var r func(*rbxapi.API, *rbxfile.Instance)
	r = func(services *rbxapi.API, obj *rbxfile.Instance) {
		if services.Classes[obj.ClassName] != nil {
			obj.IsService = true
		}
		for _, child := range obj.Children {
			r(services, child)
		}
	}
	f, _ := os.Open(filepath.Join(opt.Repo, ProjectMetaDir, "services"))
	services, _ := dump.Decode(f)
	f.Close()
	r(services, datamodel)

	root := &rbxfile.Root{
		Instances: make([]*rbxfile.Instance, len(datamodel.Children)),
	}
	copy(root.Instances, datamodel.Children)
	datamodel.RemoveAll()

	f, _ = os.Create(filepath.Join(opt.Repo, "new-"+place))
	err := bin.SerializePlace(f, opt.API, root)
	f.Close()

	return err
}

func syncInEncodeRoot() error { return nil }

func getDirPlace(dir string) (place string) {
	// dir.basename + dir-meta.format
	return filepath.Base(dir) + ".rbxl"
}

func SyncInReadRepo(opt *Options, dirNames []string) error {
	if !pathIsRepo(opt.Repo) {
		return ErrNotRepo
	}

	rules, _ := getStdRules(opt)
	rules = filterRuleType(rules, SyncIn)

	fmt.Println("RULES:", len(rules))
	for _, r := range rules {
		fmt.Printf("\t%s\n", r)
	}

	if len(dirNames) == 0 {
		dirNames = getDirsInRepo(opt.Repo)
	}
	if len(dirNames) == 0 {
		return ErrNoFiles
	}

	type dir struct {
		name    string
		place   string
		sources SourceCache
		actions []InAction
		refs    map[string]*rbxfile.Instance
	}

	dirs := make([]dir, 0, len(dirNames))
	errs := make(ErrsFile, 0, len(dirNames))

	for _, name := range dirNames {
		d := dir{
			name:    name,
			place:   getDirPlace(name),
			sources: SourceCache{},
			refs:    map[string]*rbxfile.Instance{},
		}
		var err error
		d.actions, err = syncInReadDir(opt, d.sources, name, []string{}, rules, d.refs)
		if err != nil {
			errs = append(errs, &ErrFile{FileName: name, Action: "syncing", Errors: []error{err}})
			continue
		}
		d.actions = syncInAnalyzeActions(d.actions)
		dirs = append(dirs, d)
	}

	for _, dir := range dirs {
		err := syncInVerifyActions(opt, dir.name, dir.place, dir.refs, dir.sources, dir.actions)
		if err != nil {
			errs = append(errs, &ErrFile{FileName: dir.name, Action: "syncing", Errors: []error{err}})
			continue
		}
	}

	for _, dir := range dirs {
		err := syncInApplyActions(opt, dir.name, dir.place, dir.refs, dir.sources, dir.actions)
		if err != nil {
			errs = append(errs, &ErrFile{FileName: dir.name, Action: "syncing", Errors: []error{err}})
			continue
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}
