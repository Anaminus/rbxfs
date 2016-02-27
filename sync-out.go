package rbxfs

import (
	"errors"
	"fmt"
	"github.com/robloxapi/rbxdump"
	"github.com/robloxapi/rbxfile"
	"github.com/robloxapi/rbxfile/bin"
	"github.com/robloxapi/rbxfile/xml"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const ProjectMetaDir = ".rbxfs"
const RulesFileName = "rules"

func pathIsRepo(path string) bool {
	if _, err := os.Stat(filepath.Join(path, ProjectMetaDir)); os.IsNotExist(err) {
		return false
	}
	return true
}

func parseRules(opt *Options, depth int, path string) ([]rulePair, error) {
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

func globalRulePath() string {
	// $APPDATA/rbxfs/{RulesFileName}
	return ""
}

func projectRulePath(path string) string {
	return filepath.Join(path, ProjectMetaDir, RulesFileName)
}

func filterRuleType(rules []rulePair, typ SyncType) (out []rulePair) {
	for _, rule := range rules {
		if rule.SyncType == typ {
			out = append(out, rule)
		}
	}
	return
}

func getPlacesInRepo(repo string) []string {
	files, err := ioutil.ReadDir(repo)
	if err != nil {
		return nil
	}
	s := []string{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		switch filepath.Ext(file.Name()) {
		case ".rbxm", ".rbxmx", ".rbxl", ".rbxlx":
			s = append(s, file.Name())
		}
	}
	return s
}

func syncOutReadObject(opt *Options, obj *rbxfile.Instance, dir []string, rules []rulePair) (actions []OutAction, err error) {
	defs := opt.RuleDefs
	if defs == nil {
		defs = DefaultRuleDefs
	}

	children := []int{}
	names := []string{}
	for _, pair := range rules {
		om, err := defs.CallOut(opt, pair, obj)
		if err != nil {
			//ERROR:
			return nil, err
		}
		for _, m := range om {
			if m.File.IsDir {
				for _, s := range m.Selection {
					if s.Object == obj && len(s.Children) == 1 && s.Children[0] < len(obj.Children) {
						children = append(children, s.Children[0])
						names = append(names, m.File.Name)
					}
				}
			}
			actions = append(actions, OutAction{
				Depth: pair.Depth,
				Dir:   dir,
				Map:   m,
			})
		}
	}

	for i, index := range children {
		name := names[i]
		child := obj.Children[index]
		subdir := make([]string, len(dir)+1)
		copy(subdir, dir)
		subdir[len(subdir)-1] = name
		oa, err := syncOutReadObject(opt, child, subdir, rules)
		if err != nil {
			//ERROR:
			// context: object that caused error
			return nil, err
		}
		actions = append(actions, oa...)
	}

	// for _, child := range obj.Children {
	// 	subdir := make([]string, len(dir)+1)
	// 	copy(subdir, dir)
	// 	subdir[len(subdir)-1] = child.Name()
	// 	om, err := syncOutReadObject(opt, child, subdir, rules)
	// 	if err != nil {
	// 		//ERROR:
	// 		// context: object that caused error
	// 		return nil, err
	// 	}
	// 	actions = append(actions, om...)
	// }
	return actions, nil
}

func decodePlaceFile(name string, api *rbxdump.API) (root *rbxfile.Root, err error) {
	model := false
	switch ext := filepath.Ext(name); ext {
	case ".rbxm", ".rbxmx":
		model = true
		fallthrough
	case ".rbxl", ".rbxlx":
		c := bin.RobloxCodec{
			ExcludeInvalidAPI: false,
		}
		if model {
			c.Mode = bin.ModeModel
		} else {
			c.Mode = bin.ModePlace
		}
		s := bin.Serializer{
			Decoder: c,
			Encoder: c,
			DecoderXML: xml.RobloxCodec{
				ExcludeExternal:   false,
				ExcludeInvalidAPI: false,
				ExcludeReferent:   false,
			},
		}

		place, err := os.Open(name)
		if err != nil {
			//ERROR:
			return nil, err
		}
		defer place.Close()

		root, err := s.Deserialize(place, api)
		if err != nil {
			//ERROR:
			return nil, err
		}
		return root, err
	}
	//ERROR:
	return nil, errors.New("unsupported file type for " + name)
}

func syncOutReadPlace(opt *Options, place string, dir string, rules []rulePair) (root *rbxfile.Root, actions []OutAction, err error) {
	root, err = decodePlaceFile(filepath.Join(opt.Repo, place), opt.API)
	if err != nil {
		//ERROR:
		return
	}

	datamodel := rbxfile.NewInstance("DataModel", nil)
	for i, obj := range root.Instances {
		datamodel.AddChildAt(i, obj)
	}

	actions, err = syncOutReadObject(opt, datamodel, []string{dir}, rules)

	// for _, obj := range root.Instances {
	// 	oa, err := syncOutReadObject(opt, obj, []string{dir}, rules)
	// 	if err != nil {
	// 		//ERROR:
	// 		// context: object that caused error
	// 		return nil, nil, err
	// 	}
	// 	actions = append(actions, oa...)
	// }
	return
}

type OrderedOutAction struct {
	Priority int
	Action   OutAction
}

type SortOutActionsByDepth []OrderedOutAction

func (s SortOutActionsByDepth) Len() int {
	return len(s)
}
func (s SortOutActionsByDepth) Less(i, j int) bool {
	if s[i].Action.Depth == s[j].Action.Depth {
		return s[i].Priority < s[j].Priority
	}
	return s[i].Action.Depth < s[j].Action.Depth
}
func (s SortOutActionsByDepth) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

type SortOutActionsByDir []OutAction

func (s SortOutActionsByDir) Len() int {
	return len(s)
}
func (s SortOutActionsByDir) Less(i, j int) bool {
	return getOutActionPath(s[i], 0) < getOutActionPath(s[j], 0)
}
func (s SortOutActionsByDir) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func getChildIndex(obj *rbxfile.Instance) int {
	parent := obj.Parent()
	if parent != nil {
		for i, child := range parent.Children {
			if child == obj {
				return i
			}
		}
	}
	return 0
}

type SortOutSelections []OutSelection

func (s SortOutSelections) Len() int {
	return len(s)
}
func (s SortOutSelections) Less(i, j int) bool {
	return getChildIndex(s[i].Object) < getChildIndex(s[j].Object)
}
func (s SortOutSelections) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func getOutActionPathMaxDepth(action OutAction) int {
	return len(action.Dir) + 1
}

func getOutActionPath(action OutAction, depth int) string {
	if depth == 0 {
		dir := filepath.Join(action.Dir...)
		return filepath.Join(dir, action.Map.File.Name)
	}
	if depth > len(action.Dir) {
		depth = len(action.Dir)
	}
	return filepath.Join(action.Dir[:len(action.Dir)-depth+1]...)
}

func getDirOutActionObject(action OutAction) *rbxfile.Instance {
	if len(action.Map.Selection) != 1 {
		return nil
	}
	sel := action.Map.Selection[0]
	if len(sel.Children) != 1 {
		return nil
	}
	return sel.Object.Children[sel.Children[0]]
}

func syncOutAnalyzeActions(actions []OutAction) []OutAction {
	// Valid Directory pass: Filter out actions that are not valid for
	// creating directories.
	{
		out := make([]OutAction, 0, len(actions))
		for _, action := range actions {
			if action.Map.File.IsDir && getDirOutActionObject(action) == nil {
				continue
			}
			out = append(out, action)
		}
		actions = out
	}

	// Conflicting Directory pass: Filter out directories that are created
	// multiple times.
	{
		type dirItem struct {
			conflict bool
			obj      *rbxfile.Instance
			child    int
		}

		// Mark conflicting directories. Actions do not conflict if they
		// create the same directory from the same object.
		dirs := map[string]dirItem{}
		for _, action := range actions {
			if action.Map.File.IsDir {
				path := getOutActionPath(action, 0)
				//TODO: assumes dir creation -> single selected object
				if item, ok := dirs[path]; ok {
					if action.Map.Selection[0].Object != item.obj ||
						action.Map.Selection[0].Children[0] != item.child {
						item.conflict = true
						dirs[path] = item
					}
				} else {
					dirs[path] = dirItem{
						conflict: false,
						obj:      action.Map.Selection[0].Object,
						child:    action.Map.Selection[0].Children[0],
					}
				}
			}
		}
		// Remove all conflicting directories, including any actions involving
		// their subdirectories.
		out := make([]OutAction, 0, len(actions))
	Dir:
		for _, action := range actions {
			max := getOutActionPathMaxDepth(action)
			for i := 0; i < max; i++ {
				path := getOutActionPath(action, i)
				if item, ok := dirs[path]; ok && item.conflict {
					continue Dir
				}
			}
			out = append(out, action)
		}
		actions = out
	}

	// Conflicting Action pass: Resolve multiple actions selecting the same
	// item. Also separates actions into individual selections.
	{
		type childItem struct {
			obj   *rbxfile.Instance
			child int
		}
		type propItem struct {
			obj  *rbxfile.Instance
			prop string
		}

		children := map[childItem]OrderedOutAction{}
		properties := map[propItem]OrderedOutAction{}
		for i, action := range actions {
			for _, sel := range action.Map.Selection {
				for _, child := range sel.Children {
					item := childItem{sel.Object, child}
					// Latter actions override former actions.
					if a, ok := children[item]; ok && a.Action.Depth > action.Depth {
						// Do not override if existing action is deeper.
						continue
					}
					children[item] = OrderedOutAction{
						Priority: i,
						Action: OutAction{
							Depth: action.Depth,
							Dir:   action.Dir,
							Map: OutMap{
								File: action.Map.File,
								Selection: []OutSelection{{
									Object:   sel.Object,
									Children: []int{child},
								}},
							},
						},
					}
				}
				for _, prop := range sel.Properties {
					properties[propItem{sel.Object, prop}] = OrderedOutAction{
						Priority: i,
						Action: OutAction{
							Depth: action.Depth,
							Dir:   action.Dir,
							Map: OutMap{
								File: action.Map.File,
								Selection: []OutSelection{{
									Object:     sel.Object,
									Properties: []string{prop},
								}},
							},
						},
					}
				}
			}
		}

		sorted := make([]OrderedOutAction, 0, len(children)+len(properties))
		for _, action := range children {
			sorted = append(sorted, action)
		}
		for _, action := range properties {
			sorted = append(sorted, action)
		}
		sort.Sort(SortOutActionsByDepth(sorted))

		actions = make([]OutAction, len(sorted))
		for i, action := range sorted {
			actions[i] = action.Action
		}
	}

	// Subdirectory pass: Remove actions applying to subdirs of an object that
	// doesn't become a directory.
	{
		dirs := map[string]bool{}
		for _, action := range actions {
			if action.Map.File.IsDir {
				dirs[getOutActionPath(action, 0)] = true
			}
		}
		out := make([]OutAction, 0, len(actions))
	Subdir:
		for _, action := range actions {
			for i := 1; i < getOutActionPathMaxDepth(action)-1; i++ {
				if !dirs[getOutActionPath(action, i)] {
					continue Subdir
				}
			}
			out = append(out, action)
		}
		actions = out
	}

	// Merge pass 1: Combine selections of actions that apply to the same file,
	// such that there is one action per file.
	{
		combine := map[string]*OutAction{}
		for i := 0; i < len(actions); i++ {
			action := actions[i]
			path := getOutActionPath(action, 0)
			if c, ok := combine[path]; ok {
				c.Map.Selection = append(c.Map.Selection, action.Map.Selection...)
			} else {
				combine[path] = &action
			}
		}

		out := make([]OutAction, 0, len(actions))
		for _, action := range combine {
			out = append(out, *action)
		}
		actions = out
	}

	// Merge pass 2: Combine selections within each action that apply to the
	// same object.
	{
		for i, action := range actions {
			combine := map[*rbxfile.Instance]*OutSelection{}
			for i := 0; i < len(action.Map.Selection); i++ {
				sel := action.Map.Selection[i]
				obj := sel.Object
				if c, ok := combine[obj]; ok {
					c.Children = append(c.Children, sel.Children...)
					c.Properties = append(c.Properties, sel.Properties...)
				} else {
					combine[obj] = &sel
				}
			}
			action.Map.Selection = make([]OutSelection, 0, len(combine))
			for _, sel := range combine {
				action.Map.Selection = append(action.Map.Selection, *sel)
			}
			actions[i] = action
		}
	}

	// Sort pass: Sort actions, selections, and items.
	{
		for i, action := range actions {
			for i, sel := range action.Map.Selection {
				sort.Ints(sel.Children)
				sort.Strings(sel.Properties)
				action.Map.Selection[i] = sel
			}
			sort.Sort(SortOutSelections(action.Map.Selection))
			actions[i] = action
		}
		sort.Sort(SortOutActionsByDir(actions))
	}

	return actions
}

func syncOutVerifyActions(opt *Options, place, out string, root *rbxfile.Root, actions []OutAction) error {
	return nil
}

func syncOutApplyActions(opt *Options, place, out string, root *rbxfile.Root, actions []OutAction) error {
	fmt.Printf("sync-out `%s` -> `%s`\n", filepath.Join(opt.Repo, place), filepath.Join(opt.Repo, out))
	for i, action := range actions {
		dir := filepath.Join(action.Dir...)
		path := filepath.Join(dir, action.Map.File.Name)
		var typ string
		if action.Map.File.IsDir {
			typ = "dir "
		} else {
			typ = "file"
		}
		var sel []string
		for _, s := range action.Map.Selection {
			sel = append(sel, fmt.Sprintf("{obj: %p; C: %v; P: %v}",
				s.Object, s.Children, s.Properties,
			))
		}
		fmt.Printf("\t%4d %d; %s: %-43s; sel(%02d): {%s}\n", i, action.Depth, typ, path, len(action.Map.Selection), strings.Join(sel, "; "))
	}
	return nil
}

type Options struct {
	Repo     string
	RuleDefs *FuncDef
	API      *rbxdump.API
}

func getOutDir(place string) string {
	b := filepath.Base(place)
	return filepath.Join(filepath.Dir(place), b[:len(b)-len(filepath.Ext(place))])
}

func SyncOutReadRepo(opt *Options) error {
	repo := opt.Repo
	if !pathIsRepo(repo) {
		//ERROR:
		return errors.New("not a repo")
	}

	var rules []rulePair

	r, err := parseRules(opt, 1, globalRulePath())
	if err != nil {
		//ERROR:
		fmt.Println("global rules:", err)
	} else {
		rules = append(rules, r...)
	}

	r, err = parseRules(opt, 2, projectRulePath(repo))
	if err != nil {
		//ERROR:
		fmt.Println("project rules:", err)
	} else {
		rules = append(rules, r...)
	}

	rules = filterRuleType(rules, SyncOut)

	fmt.Println("RULES:", len(rules))
	for _, r := range rules {
		fmt.Printf("\t%#v\n", r)
	}

	places := getPlacesInRepo(opt.Repo)
	out := make([]string, len(places))
	roots := make([]*rbxfile.Root, len(places))
	actions := make([][]OutAction, len(places))
	for i, place := range places {
		out[i] = getOutDir(place)
		root, a, err := syncOutReadPlace(opt, place, out[i], rules)
		if err != nil {
			//ERROR:
			fmt.Println("ERROR", err)
			continue
		}
		roots[i] = root
		actions[i] = syncOutAnalyzeActions(a)
	}

	// for i, place := range places {
	// 	err := syncOutVerifyActions(opt, place, out[i], roots[i], actions[i])
	// 	if err != nil {
	// 		//ERROR:
	// 		continue
	// 	}
	// }

	for i, place := range places {
		err := syncOutApplyActions(opt, place, out[i], roots[i], actions[i])
		if err != nil {
			//ERROR:
			continue
		}
	}

	return nil
}

func basename(name string) string {
	b := filepath.Base(name)
	return b[:len(b)-len(filepath.Ext(name))]
}

func syncOutObject(dir string, i int, obj *rbxfile.Instance) {
	name := obj.Name()
	d := filepath.Join(dir, name)
	if err := os.Mkdir(d, 0666); err != nil {
		fmt.Println("failed to add object", obj.GetFullName())
		return
	}
	for i, ch := range obj.Children {
		syncOutObject(d, i, ch)
	}
}

func syncOutFile(dir string, name string, model bool) {
	c := bin.RobloxCodec{
		ExcludeInvalidAPI: false,
	}
	if model {
		c.Mode = bin.ModeModel
	} else {
		c.Mode = bin.ModePlace
	}
	cx := xml.RobloxCodec{
		ExcludeExternal:   false,
		ExcludeInvalidAPI: false,
		ExcludeReferent:   false,
	}
	s := bin.Serializer{
		Decoder:    c,
		Encoder:    c,
		DecoderXML: cx,
	}

	place, _ := os.Open(name)
	root, err := s.Deserialize(place, nil)
	place.Close()
	if err != nil {
		fmt.Println("Decode error", err)
		return
	}

	d := filepath.Join(dir, basename(name))
	if err := os.Mkdir(d, 0666); err != nil {
		fmt.Println("failed to add root", name)
	}
	fmt.Println("added root", d)
	for i, obj := range root.Instances {
		syncOutObject(d, i, obj)
	}
}

func SyncOutFn() {
	files, _ := ioutil.ReadDir(".")
	for _, fi := range files {
		if !fi.IsDir() {
			model := false
			switch filepath.Ext(fi.Name()) {
			case ".rbxm", ".rbxmx":
				model = true
				fallthrough
			case ".rbxl", ".rbxlx":
				fmt.Println("found", fi.Name())
				syncOutFile(".", fi.Name(), model)
			}
		}
	}
}
