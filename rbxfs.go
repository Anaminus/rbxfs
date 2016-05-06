package rbxfs

import (
	"errors"
	"fmt"
	"github.com/robloxapi/rbxapi"
	"io/ioutil"
	"os"
	"path/filepath"
)

const ProjectMetaDir = ".rbxfs"
const RulesFileName = "rules"

var ErrNoFiles = errors.New("no files to sync")
var ErrNotRepo = errors.New("directory is not a repository")

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

// ErrMux combines multiple errors into a single error. If there is more than
// one error, then the amount is displayed after the message of the first
// error.
type ErrMux []error

func (err ErrMux) Error() string {
	if len(err) == 0 {
		return "no errors"
	}
	if len(err) == 1 {
		return err[0].Error()
	}
	if len(err) == 2 {
		return fmt.Sprintf("%s (1 more error)", err[0].Error())
	}
	return fmt.Sprintf("%s (%d more errors)", err[0].Error(), len(err)-1)
}

// ErrFile is an error containing a number of sub-errors related to a single
// file.
type ErrFile struct {
	FileName string
	Action   string
	Errors   []error
}

func (err ErrFile) Error() string {
	action := err.Action
	if action == "" {
		action = "reading"
	}
	if len(err.Errors) == 1 {
		return fmt.Sprintf("error when %s file %q: %s", err.Action, err.FileName, err.Errors[0])
	}
	return fmt.Sprintf("%d errors when %s file %q", len(err.Errors), err.Action, err.FileName)
}

// ErrsFile is an error containing a number of *ErrFile items.
type ErrsFile []*ErrFile

func (err ErrsFile) Error() string {
	if len(err) == 1 {
		return err[0].Error()
	}
	return fmt.Sprintf("one or more errors on %d files", len(err))
}
