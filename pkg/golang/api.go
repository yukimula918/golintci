package golang

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/packages"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func LoadBaseFile(srcFile string) (*SrcFile, error) {
	// 1. validate the input and get its source file directory
	if _, fileErr := os.Stat(srcFile); os.IsNotExist(fileErr) {
		return nil, fileErr
	} else if !strings.HasSuffix(srcFile, GoFileSuffix) {
		return nil, fmt.Errorf("not go file: %s", srcFile)
	}
	var srcPath, _ = filepath.Abs(srcFile)
	var dirPath = filepath.Clean(filepath.Dir(srcPath))

	// 2. read the source code and parse the syntax tree
	var bytes, readErr = os.ReadFile(srcPath)
	if readErr != nil {
		return nil, readErr
	}
	var fileSet = token.NewFileSet()
	syntax, parseErr := parser.ParseFile(fileSet, srcPath, nil, parser.ParseComments)
	if parseErr != nil {
		return nil, parseErr
	}
	if syntax == nil {
		return nil, fmt.Errorf("cannot parse: %s", srcPath)
	}

	// 3. perform the types checking on the syntax tree
	typeConfig := &types.Config{
		Context:                  types.NewContext(),
		IgnoreFuncBodies:         false,
		FakeImportC:              false,
		Error:                    func(err error) { /* do nothing */ },
		Importer:                 importer.Default(), // GOROOT types
		Sizes:                    nil,
		DisableUnusedImportCheck: false,
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Instances:  make(map[*ast.Ident]types.Instance),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Implicits:  make(map[ast.Node]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Scopes:     make(map[ast.Node]*types.Scope),
		InitOrder:  nil,
	}

	// 4. generate the types.Package
	typePkg, typeErr := typeConfig.Check(dirPath, fileSet, []*ast.File{syntax}, info)
	if typeErr != nil {
		// ignore the type error and return a source file with incomplete types
	} else if typePkg == nil {
		return nil, fmt.Errorf("cannot get the types.Package: %s", dirPath)
	}

	// 5. construct the *Package and the only *SrcFile for output
	pkg := newPackage(nil, syntax.Name.Name, dirPath, dirPath)
	file := pkg.newSrcFile(srcPath)
	fileErr := file.update(string(bytes), syntax, nil)
	if fileErr != nil {
		return nil, fileErr
	}
	return file, nil
}

func FindPackagePath(dirPath string) string {
	return findPackagePath(dirPath)
}

// LoadOneFile parses the AST of source file and its corresponding package info.
func LoadOneFile(srcFile string) (*ast.File, *packages.Package, error) {
	// 1. validate the input file path
	if _, fileErr := os.Stat(srcFile); os.IsNotExist(fileErr) {
		return nil, nil, fmt.Errorf("undef file: %s", srcFile)
	} else if !strings.HasSuffix(srcFile, ".go") {
		return nil, nil, fmt.Errorf("undef file: %s", srcFile)
	}
	var srcPath, _ = filepath.Abs(srcFile)
	var srcDir = filepath.Clean(filepath.Dir(srcPath))

	// 2. initialize the config and perform the parse
	fileSet := token.NewFileSet()
	loadConf := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles |
			packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedSyntax,
		Dir:   srcDir,
		Fset:  fileSet,
		Tests: true,
	}
	loadPkgs, loadErr := packages.Load(loadConf, srcDir)
	if loadErr != nil {
		return nil, nil, loadErr
	}

	// 3. find the right syntax tree to load and return
	for _, loadPkg := range loadPkgs {
		if loadPkg == nil || len(loadPkg.Syntax) == 0 {
			continue
		}
		for _, syntax := range loadPkg.Syntax {
			var pos = loadPkg.Fset.Position(syntax.Pos())
			if pos.Filename == srcPath {
				return syntax, loadPkg, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("cannot parse: %s", srcPath)
}

// LoadOnePkg simply load the syntax tree and type info of source files in the directory
// and return its package (as object of packages.Package).
//
// Note that: this
func LoadOnePkg(srcDir string) (*packages.Package, error) {
	// 1. initialize the config and data for loading
	fileSet := token.NewFileSet()
	loadConf := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles |
			packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedSyntax,
		Dir:   srcDir,
		Fset:  fileSet,
		Tests: true,
	}

	// 2. parse the AST and load its type information
	loadPkgs, loadErr := packages.Load(loadConf, srcDir)
	if loadErr != nil {
		return nil, loadErr
	}
	var resultPkgs []*packages.Package
	for _, loadPkg := range loadPkgs {
		if loadPkg != nil {
			resultPkgs = append(resultPkgs, loadPkg)
		}
	}

	// 3. check the validity of output and return one
	if len(resultPkgs) != 1 {
		return nil, fmt.Errorf("cannot generate: %d", len(resultPkgs))
	} else {
		return resultPkgs[0], nil
	}
}

// LoadAllPkg will parse the AST of all source files under the directory and
// load the type & package information.
func LoadAllPkg(srcDir string) ([]*packages.Package, error) {
	// 1. collect the set of directories with source files
	var pkgToSrcFiles = make(map[string][]string)
	_ = filepath.Walk(srcDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			dir := filepath.Dir(path)
			pkgToSrcFiles[dir] = append(pkgToSrcFiles[dir], path)
		}
		return nil
	})
	var pkgDirs []string
	for pkgPath, srcFiles := range pkgToSrcFiles {
		if len(srcFiles) > 0 {
			pkgDirs = append(pkgDirs, pkgPath)
		}
	}

	// 2. initialize the config and parse AST packages
	fileSet := token.NewFileSet()
	loadConf := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles |
			packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedSyntax,
		Dir:   srcDir,
		Fset:  fileSet,
		Tests: true,
	}
	loadPkgs, loadErr := packages.Load(loadConf, srcDir)
	if loadErr != nil {
		return nil, loadErr
	}

	// 3. collect the output packages and return them if any
	var resultPkgs []*packages.Package
	for _, loadPkg := range loadPkgs {
		if loadPkg != nil {
			resultPkgs = append(resultPkgs, loadPkg)
		}
	}
	return resultPkgs, nil
}
