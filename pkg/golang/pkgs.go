// Package golang implements the model to load and represent syntax and semantic information from
// source code in the .go files.
//
// Specifically, this file defines Package, which provides interfaces to load and access the type,
// syntax and semantic information of every source files under the package of being analyzed.
//
// Package can be seen as the basic element taken into account for static analyzers in go-linters.
package golang

import (
	"go/token"
	"go/types"
	"time"
)

// Package represents a package with its source files (modeled as SrcFile) being loaded from code.
//
// Package is the basic element for static analyzer to taken as inputs (concurrently). The Package
// could be loaded with the syntactic, type's and semantic information of code at different levels.
type Package struct {
	program *Program // program is the parent object where this Package is created
	pkgName string   // pkgName is the short name to refer this from the code of other packages
	pkgPath string   // pkgPath is logical path to import this package in file of other package
	dirPath string   // dirPath is the absolute path of directory of this package's source file

	loadInfo *LoadInfo           // loadInfo records the information of last loading of this package
	srcFiles map[string]*SrcFile // srcFiles map from absolute path to the corresponding source file

	fileSet *token.FileSet // fileSet positions the syntax and semantic element in source files
	imports []string       // imports are the set of logical paths of packages imported in this package
	typePkg *types.Package // typePkg declares the package
	typInfo *types.Info    // typInfo records the types and declarations of any variable and expression
	typSize *types.Sizes   // typSize records the size of bytes hold by any type in this package
}

// LoadInfo records the information of the last loading a package, including the syntactic, types
// and the other error information that might be used for debugging and analyzing.
type LoadInfo struct {
	LoadTime     time.Time // LoadTime is the time this loading is executed
	LoadedFiles  []string  // LoadedFiles are paths of source files loaded
	IgnoredFiles []string  // IgnoredFiles are paths of those not be loaded

	IllTyped   bool    // IllTyped is true if any type error occurs in parsing
	FileErrors []error // FileErrors are a set of errors when parsing the file
	TypeErrors []error // TypeErrors are a set of errors in checking the types
	DepsErrors []error // DepsErrors are a set of errors in dependency imports
}

// newPackage creates a new package in the program given its name, logical path and directory path.
func newPackage(program *Program, pkgName, pkgPath, dirPath string) *Package {
	return &Package{
		program:  program,
		pkgName:  pkgName,
		pkgPath:  pkgPath,
		dirPath:  dirPath,
		loadInfo: nil,
		srcFiles: make(map[string]*SrcFile),
		fileSet:  nil,
		imports:  nil,
		typePkg:  nil,
		typInfo:  nil,
		typSize:  nil,
	}
}

// Program is the parent object where this Package is created
func (pkg *Package) Program() *Program {
	if pkg != nil {
		return pkg.program
	}
	return nil
}

// PkgName is the short name to refer this from the code of other packages
func (pkg *Package) PkgName() string {
	if pkg != nil {
		return pkg.pkgName
	}
	return NoneString
}

// PkgPath is logical path to import this package in file of other package
func (pkg *Package) PkgPath() string {
	if pkg != nil {
		return pkg.pkgPath
	}
	return NoneString
}

// DirPath is the absolute path of directory of this package's source file
func (pkg *Package) DirPath() string {
	if pkg != nil {
		return pkg.dirPath
	}
	return NoneString
}

// IsLoaded check whether this package is loaded with any syntax, type and semantic information of
// its source files.
func (pkg *Package) IsLoaded() bool {
	if pkg != nil {
		return pkg.loadInfo != nil
	}
	return false
}

// LoadInfo records the information of the latest loading for this package
func (pkg *Package) LoadInfo() *LoadInfo {
	if pkg != nil {
		return pkg.loadInfo
	}
	return nil
}

// GoFiles are the set of absolute paths of source files in this package
func (pkg *Package) GoFiles() []string {
	if pkg != nil {
		var paths []string
		for path, file := range pkg.srcFiles {
			if file != nil && len(path) > 0 {
				paths = append(paths, path)
			}
		}
		return paths
	}
	return nil
}

// SrcFile returns the source file w.r.t. the absolute file in this package
func (pkg *Package) SrcFile(path string) *SrcFile {
	if pkg != nil {
		return pkg.srcFiles[path]
	}
	return nil
}

// FileSet positions the syntax and semantic element in its source files
func (pkg *Package) FileSet() *token.FileSet {
	if pkg != nil {
		return pkg.fileSet
	}
	return nil
}

// Imports are the set of logical paths of packages imported in this package
func (pkg *Package) Imports() []string {
	if pkg != nil {
		return pkg.imports
	}
	return nil
}

// TypePkg declares the package and its types
func (pkg *Package) TypePkg() *types.Package {
	if pkg != nil {
		return pkg.typePkg
	}
	return nil
}

// TypeInfo records the types and declarations of any variable and expression
func (pkg *Package) TypeInfo() *types.Info {
	if pkg != nil {
		return pkg.typInfo
	}
	return nil
}

// TypeSize records the size of bytes hold by any type in this package
func (pkg *Package) TypeSize() *types.Sizes {
	if pkg != nil {
		return pkg.typSize
	}
	return nil
}

// newSrcFile creates a SrcFile representing the source file in the package
func (pkg *Package) newSrcFile(srcPath string) *SrcFile {
	if pkg != nil {
		if _, ok := pkg.srcFiles[srcPath]; !ok {
			pkg.srcFiles[srcPath] = newSrcFile(pkg, srcPath)
		}
		return pkg.srcFiles[srcPath]
	}
	return nil
}
