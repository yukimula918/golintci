// Package golang implements the model to load and represent syntax and semantic information from
// source code in the .go files.
//
// Specifically, this file implements the top-level model Program that provides the packages and
// source files for static analyzers to consume.
package golang

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Module gives the information in `go.mod` file that defines the module of project be analyzed.
type Module struct {
	RootPath     string            // RootPath is the absolute path of root directory of repository
	GoVersion    string            // GoVersion is the version of go language required in `go.mod`
	GoModFile    string            // GoModFile is the absolute path of go.mod file of the project
	ModuleName   string            // ModuleName is the name declared in go.mod file
	DirectDeps   map[string]string // DirectDeps map from dependency packages to required versions
	IndirectDeps map[string]string // IndirectDeps model those indirectly dependency packages info
}

// newModule returns the Module information read from the path of go.mod as given.
func newModule(goModFile string) (*Module, error) {
	// 1. check the existence of input 'go.mod' file
	if _, err := os.Stat(goModFile); os.IsNotExist(err) {
		return nil, err
	} else if !strings.HasSuffix(goModFile, GoModFileName) {
		return nil, fmt.Errorf("not go.mod: %s", goModFile)
	}

	// 2. read the lines of text from 'go.mod' file
	goModFile, _ = filepath.Abs(goModFile)
	var bytes, err = os.ReadFile(goModFile)
	if err != nil {
		return nil, err
	} else if len(bytes) == 0 {
		return nil, fmt.Errorf("empty file: %s", goModFile)
	}
	lines := strings.Split(string(bytes), NewLine)
	module := &Module{
		RootPath:     filepath.Dir(goModFile),
		GoVersion:    "",
		GoModFile:    goModFile,
		ModuleName:   "",
		DirectDeps:   make(map[string]string),
		IndirectDeps: make(map[string]string),
	}

	// 3. construct the go.mod lines in the Module
	for _, line := range lines {
		if strings.HasPrefix(line, ModulePrefix) {
			module.ModuleName = strings.TrimSpace(line[len(ModulePrefix):])
		} else if strings.HasPrefix(line, VersionPrefix) {
			module.GoVersion = strings.TrimSpace(line[len(VersionPrefix):])
		} else if strings.HasPrefix(line, TabString) {
			items := strings.Split(strings.TrimSpace(line), SpaceChar)
			if len(items) >= 2 {
				depPkgPath := strings.TrimSpace(items[0])
				depVersion := strings.TrimSpace(items[1])
				lastItem := strings.TrimSpace(items[len(items)-1])
				if lastItem == GoModIndirect {
					module.IndirectDeps[depPkgPath] = depVersion
				} else {
					module.DirectDeps[depPkgPath] = depVersion
				}
			}
		}
	}
	return module, nil
}

// Program defines the top-level model of packages that will be taken as input by static analyzers.
type Program struct {
	pkgSet map[string]*Package // pkgSet is the set of packages loaded in this program
	module *Module             // module record the information in `go.mod` of program
}

// goModFileOf returns absolute path of 'go.mod' in current work directory (cwd).
func goModFileOf(cwd string) (string, error) {
	cwdPath, _ := filepath.Abs(cwd)
	for len(cwdPath) > 0 && cwdPath != "/" && cwdPath != "." && cwdPath != ".." {
		goModFile := filepath.Join(cwdPath, GoModFileName)
		if _, err := os.Stat(goModFile); !os.IsNotExist(err) {
			return cwdPath, nil
		}
		cwdPath = filepath.Dir(cwdPath)
	}
	return "", fmt.Errorf("couldn't find `go.mod` file from: %s", cwd)
}

// initProgram returns initialized Program with module info, or nil if it fails to load the module.
func initProgram(cwd string) (*Program, error) {
	// 1. infer the absolute path of CWD
	if len(cwd) == 0 {
		newCwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		cwd = newCwd
	}
	cwdPath, _ := filepath.Abs(cwd)

	// 2. find the 'go.mod' from CWD path
	goModFile, err := goModFileOf(cwdPath)
	if err != nil {
		return nil, err
	}
	module, err := newModule(goModFile)
	if err != nil {
		return nil, err
	}
	if module == nil {
		return nil, fmt.Errorf("can't create Module: %s", goModFile)
	}

	// 3. return the initialized Program instance
	return &Program{
		pkgSet: make(map[string]*Package),
		module: module,
	}, nil
}

// AllPackages return the set of all loaded packages in the program.
func (prog *Program) AllPackages() []*Package {
	if prog != nil {
		var pkgs []*Package
		for _, pkg := range prog.pkgSet {
			if pkg != nil {
				pkgs = append(pkgs, pkg)
			}
		}
		return pkgs
	}
	return nil
}

// Module records the module information of go.mod from the program.
func (prog *Program) Module() *Module {
	if prog != nil {
		return prog.module
	}
	return nil
}

// Package return the unique package in program w.r.t. the unique path
func (prog *Program) Package(pkgPath string) *Package {
	if prog != nil {
		return prog.pkgSet[pkgPath]
	}
	return nil
}

// newPackage is an internal method to create package from the program
func (prog *Program) newPackage(pkgName, pkgPath, dirPath string) *Package {
	if prog != nil {
		if _, ok := prog.pkgSet[pkgPath]; !ok {
			prog.pkgSet[pkgPath] = newPackage(prog, pkgName, pkgPath, dirPath)
		}
		return prog.pkgSet[pkgPath]
	}
	return nil
}
