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
	for _, pair := range rules {
		jdir := filepath.Join(dir...)
		is, err := opt.RuleDefs.CallIn(opt, cache, pair, jdir)
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

func syncInAnalyzeActions(actions []InAction) []InAction {
	return actions
}

func syncInVerifyActions(opt *Options, dir, place string, cache SourceCache, actions []InAction) error {
	return nil
}
func syncInApplyActions(opt *Options, dir, place string, cache SourceCache, actions []InAction) error {
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
		fmt.Printf("\t%4d %d; %-43s; sel(%02d): {%s}\n", i, action.Depth, path, len(action.Selection), strings.Join(sel, "; "))
	}
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
		_, _ = i, dir
	}

	for i, dir := range dirs {
		err := syncInApplyActions(opt, dir, places[i], sources[i], actions[i])
		if err != nil {
			//ERROR:
			continue
		}
		_, _ = i, dir
	}

	return nil
}
