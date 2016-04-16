package rbxfs

import (
	"github.com/robloxapi/rbxapi"
	"io/ioutil"
	"os"
	"path/filepath"
)

const ProjectMetaDir = ".rbxfs"
const RulesFileName = "rules"

func pathIsRepo(path string) bool {
	if _, err := os.Stat(filepath.Join(path, ProjectMetaDir)); os.IsNotExist(err) {
		return false
	}
	return true
}

func globalRulePath() string {
	// $APPDATA/rbxfs/{RulesFileName}
	return ""
}

func projectRulePath(path string) string {
	return filepath.Join(path, ProjectMetaDir, RulesFileName)
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

func getDirsInRepo(repo string) []string {
	files, err := ioutil.ReadDir(repo)
	if err != nil {
		return nil
	}
	s := []string{}
	for _, file := range files {
		if !file.IsDir() || file.Name() == ProjectMetaDir {
			continue
		}
		s = append(s, file.Name())
	}
	return s
}

type Options struct {
	Repo     string
	RuleDefs *FuncDef
	API      *rbxapi.API
}
