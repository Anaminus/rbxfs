package rbxfs

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func syncInReadDir(opt *Options, cache SourceCache, dir []string, rules []rulePair) (actions []InAction, err error) {
	defs := opt.RuleDefs
	if defs == nil {
		defs = DefaultRuleDefs
	}

	children := map[string]bool{}
	jdir := filepath.Join(dir...)
	for _, pair := range rules {
		is, err := defs.CallIn(opt, cache, pair, jdir)
		if err != nil {
			//ERROR
			return nil, err
		}
		for _, s := range is {
			// Scan for directories.
			if source, ok := cache[filepath.Join(jdir, s.File)]; ok {
				if source.IsDir && len(s.Children) == 1 {
					children[s.File] = true
				}
			}
			actions = append(actions, InAction{
				Depth:     pair.Depth,
				Dir:       dir,
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
		subdir := make([]string, len(dir)+1)
		copy(subdir, dir)
		subdir[len(subdir)-1] = name
		a, err := syncInReadDir(opt, cache, subdir, rules)
		if err != nil {
			//ERROR
			return nil, err
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

func syncInVerifyActions(opt *Options, dir, place string, cache SourceCache, actions []InAction) error {
	fmt.Printf("sync-in `%s` -> `%s`\n", filepath.Join(opt.Repo, dir), filepath.Join(opt.Repo, place))
	for i, action := range actions {
		sub := filepath.Join(action.Dir...)
		path := filepath.Join(dir, sub)

		var sel []string
		for _, s := range action.Selection {
			sel = append(sel, fmt.Sprintf("{file: %s; I: %t; C: %v; P: %v; V: %v}",
				s.File, s.Ignore, s.Children, s.Properties, s.Values,
			))
		}
		fmt.Printf("\t%4d %d; %-32s; sel(%02d): {%s}\n", i, action.Depth, path, len(action.Selection), strings.Join(sel, "; "))
	}
	return nil
}
func syncInApplyActions(opt *Options, dir, place string, cache SourceCache, actions []InAction) error {
	return nil
}

func syncInEncodeRoot() error { return nil }

func getDirPlace(dir string) (place string) {
	// dir.basename + dir-meta.format
	return filepath.Base(dir) + ".rbxl"
}

func SyncInReadRepo(opt *Options) error {
	if !pathIsRepo(opt.Repo) {
		//ERROR:
		return errors.New("not a repo")
	}

	rules, _ := getStdRules(opt)
	rules = filterRuleType(rules, SyncIn)

	fmt.Println("RULES:", len(rules))
	for _, r := range rules {
		fmt.Printf("\t%s\n", r)
	}

	dirs := getDirsInRepo(opt.Repo)
	places := make([]string, len(dirs))
	sources := make([]SourceCache, len(dirs))
	actions := make([][]InAction, len(dirs))

	for i, dir := range dirs {
		places[i] = getDirPlace(dir)
		sources[i] = SourceCache{}
		a, err := syncInReadDir(opt, sources[i], []string{dir}, rules)
		if err != nil {
			//ERROR
			continue
		}
		actions[i] = syncInAnalyzeActions(a)
	}

	for i, dir := range dirs {
		err := syncInVerifyActions(opt, dir, places[i], sources[i], actions[i])
		if err != nil {
			//ERROR:
			continue
		}
	}

	for i, dir := range dirs {
		err := syncInApplyActions(opt, dir, places[i], sources[i], actions[i])
		if err != nil {
			//ERROR:
			continue
		}
	}

	return nil
}
