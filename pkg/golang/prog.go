// Package golang implements the model to load and represent syntax and semantic information from
// source code in the .go files.
//
// Specifically, this file implements the top-level model Program that provides the packages and
// source files for static analyzers to consume.
package golang

import "golang.org/x/tools/go/packages"

// Program defines the top-level model of packages that will be taken as input by static analyzers.
type Program struct {
	pkgSet map[string]*Package // pkgSet is the set of packages loaded in this program
	module *packages.Module    // module records the module information of its go.mod
}

// newProgram creates an empty model of program taken as inputs of the following static analyzers.
func newProgram() *Program {
	return &Program{
		pkgSet: make(map[string]*Package),
		module: nil,
	}
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
func (prog *Program) Module() *packages.Module {
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
