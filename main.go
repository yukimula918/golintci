package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yukimula918/golintci/pkg/golang"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

const (
	consText = "Hello, world!"
	constInt = 10023
)

func percent(succeed, errors int) float64 {
	ratio := 0.0
	if succeed > 0 {
		ratio = float64(succeed) / float64(succeed+errors)
		ratio = float64(int(ratio*10000)) / 100.0
	}
	return ratio
}

func main() {
	// rootDir := "/Users/linhuan/Development/GoRepos"
	// rootDir := "/Users/linhuan/Development/GoRepos/golangci-lint"
	// rootDir := "/Users/linhuan/Development/GoRepos/istio"
	// rootDir := "/Users/linhuan/Development/MyRepos/golintci"
	// testParseGoPackageFile(rootDir)
	// viewParseForGoPackages(rootDir)
	// testCompiledGoPackages(rootDir)
	// testCompileGoPackageTypes(rootDir)
	// testGoModTypePackages(rootDir)
	// testLoadTypesInGoPackages(rootDir)
	// viewObjectsAndTypesIn(rootDir)
	// viewLoadConfigPackage(rootDir)
	// viewLoadConfigAstType(rootDir)
	// testCompileForOneFile(rootDir)
	// testLoadBaseFile(rootDir)
}

func testLoadBaseFile(rootDir string) {
	var pkgToFiles = findPackagesAndGoFiles(rootDir)
	var passNumber, noneNumber = 0, 0
	for _, goFiles := range pkgToFiles {
		for _, goFile := range goFiles {
			srcFile, err := golang.LoadBaseFile(goFile)
			if err != nil || srcFile == nil {
				fmt.Printf("\t-- ERR-S: %v\n", err)
				noneNumber++
			} else {
				// fmt.Printf("\t-- PARSE: %s\n", srcFile.Path())
				passNumber++
			}
		}
	}
	fmt.Printf("Total:\t+%d; -%d; (%v).\n", passNumber, noneNumber, percent(passNumber, noneNumber))
}

// findPackagesAndGoFiles return a map from directory to the go files included.
func findPackagesAndGoFiles(rootDir string) map[string][]string {
	var goFiles []string
	_ = filepath.Walk(rootDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			goFiles = append(goFiles, path)
		}
		return nil
	})

	var pkgToFiles = make(map[string][]string)
	for _, goFile := range goFiles {
		var goDir = filepath.Dir(goFile)
		pkgToFiles[goDir] = append(pkgToFiles[goDir], goFile)
	}
	return pkgToFiles
}

// testParseForSingleFile test the pass-rate of parsing single file in rootDir.
func testParseForSingleFile(rootDir string) {
	var parseNumber, errorNumber = 0, 0
	var pkgToFiles = findPackagesAndGoFiles(rootDir)

	var begTime = time.Now()
	for _, goFiles := range pkgToFiles {
		var fileSet = token.NewFileSet()
		for _, goFile := range goFiles {
			file, err := parser.ParseFile(fileSet, goFile, nil, parser.ParseComments)
			if err != nil || file == nil {
				errorNumber++
			} else {
				parseNumber++
			}
		}
	}
	var endTime = time.Now()

	fmt.Printf("\nTaking %v seconds.", int(endTime.Sub(begTime).Seconds()))
	fmt.Printf("\nSummary: %d parsed; %d errors (%v).\n",
		parseNumber, errorNumber, percent(parseNumber, errorNumber))
}

// testParseGoPackageFile test the pass-rate of parsing files at package level.
func testParseGoPackageFile(rootDir string) {
	var parseNumber, errorNumber = 0, 0
	var pkgToFiles = findPackagesAndGoFiles(rootDir)

	var begTime = time.Now()
	for pkgDir, goFiles := range pkgToFiles {
		pkgs, err := parser.ParseDir(token.NewFileSet(), pkgDir, nil, parser.ParseComments)
		if err != nil || pkgs == nil {
			errorNumber += len(goFiles)
		} else {
			parseNumber += len(goFiles)
		}
	}
	var endTime = time.Now()

	fmt.Printf("\nTaking %v seconds.", int(endTime.Sub(begTime).Seconds()))
	fmt.Printf("\nSummary: %d parsed; %d errors (%v).\n",
		parseNumber, errorNumber, percent(parseNumber, errorNumber))
}

// viewParseForGoPackages print the information of AST parse at packages level.
func viewParseForGoPackages(rootDir string) {
	pkgToFiles := findPackagesAndGoFiles(rootDir)

	for pkgDir, goFiles := range pkgToFiles {
		var fileSet = token.NewFileSet()
		pkgs, err := parser.ParseDir(fileSet, pkgDir,
			nil, parser.ParseComments)
		if err != nil || pkgs == nil {
		}
		fmt.Printf("Package-%d: %s\n", len(pkgs), pkgDir)

		var astFileSet = make(map[string]bool)
		for pkgName, astPkg := range pkgs {
			for _, astFile := range astPkg.Files {
				var pos = fileSet.Position(astFile.Package)
				fmt.Printf("\t[%s]:\t%s:%d:%d\n",
					pkgName, pos.Filename, pos.Line, pos.Column)
				astFileSet[pos.Filename] = true
			}
		}

		for _, goFile := range goFiles {
			if !astFileSet[goFile] {
				// fmt.Printf("\tUndef: \t%s\n", goFile)
			}
		}
		fmt.Println()
	}
}

type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }

func isValidType(typ types.Type) bool {
	if typ == nil {
		return false
	}
	switch typ := typ.(type) {
	case *types.Basic:
		return typ.Kind() != types.Invalid
	default:
		return true
	}
}

func isValidExpr(expr ast.Expr) bool {
	if expr == nil {
		return false
	}

	switch expr.(type) {
	case *ast.KeyValueExpr:
		return false
	case *ast.FuncType:
		return false
	}

	return true
}

// testCompiledGoPackages test the compilation of each AST package to types.Package.
func testCompiledGoPackages(rootDir string) {
	var pkgToFiles = findPackagesAndGoFiles(rootDir)

	var pkgToSyntax = make(map[string][]*ast.File)
	var fileSet = token.NewFileSet()
	var imports = make(map[string]*types.Package)
	for pkgDir, _ := range pkgToFiles {
		pkgs, err := parser.ParseDir(fileSet, pkgDir, nil, parser.ParseComments)
		if err != nil {
			// log.Errorf("\tERR:\t%v", err)
		}
		if pkgs != nil {
			for pkgName, pkgNode := range pkgs {
				pkgKey := fmt.Sprintf("%s@%s", pkgDir, pkgName)
				for _, astFile := range pkgNode.Files {
					pkgToSyntax[pkgKey] = append(pkgToSyntax[pkgKey], astFile)
				}
				imports[pkgDir] = types.NewPackage(pkgDir, pkgName)
			}
		}
	}

	for pkgKey, astFiles := range pkgToSyntax {
		fmt.Printf("PKG:\t%s\n", pkgKey)
		items := strings.Split(pkgKey, "@")
		context := types.NewContext()

		var typesErrors []error
		config := &types.Config{
			Context:          context,
			GoVersion:        "go1.18",
			IgnoreFuncBodies: false,
			FakeImportC:      true,
			Error:            func(err error) { typesErrors = append(typesErrors, err) },
			// Importer:                 importerFunc(importer),
			Importer:                 importer.Default(),
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
		pkg, err := config.Check(items[0], fileSet, astFiles, info)
		if err != nil || pkg == nil {
			// log.Errorf("ERR: %v", err)
		}

		for _, astFile := range astFiles {
			var typeNumber, noneNumber = 0, 0
			var pos = fileSet.Position(astFile.Package)
			fmt.Printf("\t%s:%d:%d\n", pos.Filename, pos.Line, pos.Column)
			ast.Inspect(astFile, func(node ast.Node) bool {
				if node == nil {
					return false
				}
				expr, ok := node.(ast.Expr)
				if expr != nil && ok && isValidExpr(expr) {
					var pos = fileSet.Position(expr.Pos())
					var typ = info.TypeOf(expr)
					if isValidType(typ) {
						typeNumber++
						// fmt.Printf("\t--- %v: %s:%d:%d\n", typ, pos.Filename, pos.Line, pos.Column)
					} else {
						noneNumber++
						fmt.Printf("\t--- [%T]: %s:%d:%d\n", expr, pos.Filename, pos.Line, pos.Column)
					}
				}
				return true
			})

			fmt.Printf("\tTotal: %d types & %d error (%v).\n",
				typeNumber, noneNumber, percent(typeNumber, noneNumber))

			var text string
			_, _ = fmt.Scanln(&text)
		}
		fmt.Println()
	}
}

type MyImporter struct {
	FileSet  *token.FileSet
	Default  types.Importer
	Cwd      string
	Packages map[string]*types.Package
}

func newMyImporter(path string) *MyImporter {
	return &MyImporter{
		FileSet:  token.NewFileSet(),
		Default:  importer.Default(),
		Cwd:      path,
		Packages: make(map[string]*types.Package),
	}
}

func (imp *MyImporter) Import(path string) (*types.Package, error) {
	if typePkg, err := imp.Default.Import(path); err == nil {
		return typePkg, nil
	}
	// var goModDir = filepath.Join(build.Default.GOPATH, "pkg/mod", path+"@v1.9.0")
	// fmt.Println("\t\t--> ", goModDir)
	path += "@v1.9.0"
	fmt.Println("\t\t--> ", path)
	if typePkg, err := imp.Default.Import(path + "@v1.9.0"); err == nil {
		return typePkg, nil
	}
	return nil, fmt.Errorf("cannot find '%s'", path)
}

// testCompileGoPackageTypes test the rate of checking expression type in types.Package.
func testCompileGoPackageTypes(rootDir string) {
	var pkgToFiles = findPackagesAndGoFiles(rootDir)
	for pkgDir, _ := range pkgToFiles {
		// initialize the config and ast.Package
		fileSet := token.NewFileSet()
		astPkgs, err := parser.ParseDir(fileSet, pkgDir, nil, parser.ParseComments)
		if err != nil || astPkgs == nil {
			// log.Errorf("\tERR1: %s", err)
			continue
		}

		// construct the parse of ast.File for all
		for pkgName, pkgNode := range astPkgs {
			var imports []string
			var files []*ast.File
			for _, astFile := range pkgNode.Files {
				files = append(files, astFile)
				for _, importSpec := range astFile.Imports {
					path := importSpec.Path.Value
					path = strings.Trim(path, "\"")
					imports = append(imports, path)
				}
			}
			fmt.Printf("PKG: %s:%s\t%d files\n", pkgDir, pkgName, len(files))

			// typePkg := types.NewPackage(pkgName, pkgName)
			typConfig := &types.Config{
				Context:                  types.NewContext(),
				GoVersion:                "",
				IgnoreFuncBodies:         false,
				FakeImportC:              true,
				Error:                    func(err error) { /* fmt.Printf("\tERR-t: %v\n", err) */ },
				Importer:                 newMyImporter(pkgDir),
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
			typePkg, err := typConfig.Check(pkgName, fileSet, files, info)
			if err != nil || typePkg == nil {
				// log.Errorf("\tERR2: %v", err)
			}
			if typePkg != nil {
				fmt.Printf("\tPath: %s\n\tName: %s\n", typePkg.Path(), typePkg.Name())
				fmt.Printf("\tImports: %v\n", typePkg.Imports())
			}

			var typeNumber, noneNumber = 0, 0
			for _, astFile := range files {
				ast.Inspect(astFile, func(node ast.Node) bool {
					if node == nil {
						return false
					}
					expr, ok := node.(ast.Expr)
					if ok && isValidExpr(expr) {
						typ := info.TypeOf(expr)
						if isValidType(typ) {
							typeNumber++
						} else {
							noneNumber++
						}
					}
					return true
				})
			}
			fmt.Printf("\t%d types & %d error (%v).\n",
				typeNumber, noneNumber, percent(typeNumber, noneNumber))
			fmt.Println()
		}
	}
}

// testGoModTypePackages test the pass-rate of getting types of dependence packages in go.mod
func testGoModTypePackages(rootDir string) {
	var goModFile = filepath.Join(rootDir, "go.mod")
	if _, err := os.Stat(goModFile); os.IsNotExist(err) {
		panic(fmt.Sprintf("undef: %s", goModFile))
	}
	goModBytes, err := os.ReadFile(goModFile)
	if err != nil || len(goModBytes) == 0 {
		panic(fmt.Sprintf("unable to read: %s", goModFile))
	}

	var lines = strings.Split(string(goModBytes), "\n")
	var depPkgsToPaths = make(map[string]string) // pkg to src path
	var modPrefix = filepath.Join(build.Default.GOPATH, "pkg/mod")
	for _, line := range lines {
		if strings.HasPrefix(line, "\t") {
			items := strings.Split(strings.TrimSpace(line), " ")
			if len(items) >= 2 {
				depPkgPath := strings.TrimSpace(items[0])
				depPkgVersion := strings.TrimSpace(items[1])
				if len(depPkgPath) > 0 && len(depPkgVersion) > 0 {
					depPkgsToPaths[depPkgPath] = filepath.Join(modPrefix, depPkgPath+"@"+depPkgVersion)
				}
			}
		}
	}

	for pkgPath, srcPath := range depPkgsToPaths {
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			fmt.Printf("--> ERR: %s\n", pkgPath)
		} else {
			fmt.Printf("--> SRC: %s\n", srcPath)
		}
	}

}

// viewObjectsAndTypesIn view the pass-rate of getting types.Object and types.Type in project
func viewObjectsAndTypesIn(rootDir string) {
	pkgToFiles := findPackagesAndGoFiles(rootDir)
	for pkgDir := range pkgToFiles {
		// 1. parse the AST of each source code file in the package directory
		fileSet := token.NewFileSet()
		astPkgs, err := parser.ParseDir(fileSet, pkgDir, nil, parser.ParseComments)
		if err != nil || astPkgs == nil {
			continue // ignore the files that could not be correctly compiled
		}

		fmt.Printf("Parse: %s\n", pkgDir)
		for _, astPkg := range astPkgs {
			// 1. collect the AST of each source file
			var files []*ast.File
			for _, file := range astPkg.Files {
				files = append(files, file)
			}

			// 2. initialize the types.Config
			config := &types.Config{
				Importer: importer.Default(),
				Sizes: &types.StdSizes{
					WordSize: 8,
					MaxAlign: 8,
				},
			}
			info := &types.Info{
				Types:      make(map[ast.Expr]types.TypeAndValue),
				Instances:  nil,
				Defs:       make(map[*ast.Ident]types.Object),
				Uses:       make(map[*ast.Ident]types.Object),
				Implicits:  make(map[ast.Node]types.Object),
				Selections: make(map[*ast.SelectorExpr]*types.Selection),
				Scopes:     make(map[ast.Node]*types.Scope),
				InitOrder:  nil,
			}

			// 3. do the type-checking on package
			typePkg, err := config.Check(pkgDir, fileSet, files, info)
			if err != nil {
				fmt.Println("ERR-t: ", err)
				// do nothing on it
			}
			if typePkg != nil {
				/*
					fmt.Printf("\tPkg-Path: %s\n", typePkg.Path())
					fmt.Printf("\tPkg-Name: %s\n", typePkg.Name())
					fmt.Printf("\tImported: \n")
					for _, importPkg := range typePkg.Imports() {
						fmt.Printf("\t\t[%s]\t%s\n", importPkg.Name(), importPkg.Path())
					}
				*/
			}

			// 4. print the uses and defs in info
			/*
				fmt.Printf("\tDefs-Map:\n")
				for ident, object := range info.Defs {
					var pos = fileSet.Position(ident.Pos())
					fmt.Printf("\t\t[%s] %s:%d:%d\n", ident.Name,
						pos.Filename, pos.Line, pos.Column)
					switch object := object.(type) {
					case *types.PkgName:
						fmt.Printf("\t\t-- PkgName:\t%s\n", object.Imported().Path())
					case *types.Const:
						fmt.Printf("\t\t-- Const: \t%s\n", object.Val())
					case *types.Var:
						fmt.Printf("\t\t-- Var: \t%v\n", object.Type())
					case *types.Func:
						fmt.Printf("\t\t-- Func: \t%v\n", object.FullName())
					}
				}
			*/
			/*
				fmt.Printf("\tUses-Map:\n")
				for ident, object := range info.Uses {
					var pos = fileSet.Position(ident.Pos())
					fmt.Printf("\t\t[%s] %s:%d:%d\n", ident.Name,
						pos.Filename, pos.Line, pos.Column)
					fmt.Printf("\t\t-- %T:\n", object)
				}
			*/
			/*
				fmt.Printf("\tImplicits:\n")
				for node, object := range info.Implicits {
					var pos = fileSet.Position(node.Pos())
					fmt.Printf("\t\t[%T]: %s:%d:%d\n", node, pos.Filename, pos.Line, pos.Column)
					switch object := object.(type) {
					case *types.Var:
						fmt.Printf("\t\t-- Var: '%s'\t%s\n", object.Name(), object.Type().String())
					case *types.PkgName:
						fmt.Printf("\t\t-- Pkg: '%s'\t%s\n", object.Name(), object.Imported().Path())
					}
				}
			*/
			/*
				fmt.Printf("\tSelect:\n")
				for expr, selection := range info.Selections {
					var pos = fileSet.Position(expr.Pos())
					fmt.Printf("\t\t[x.%s]:\t%s:%d:%d\n", expr.Sel.Name, pos.Filename, pos.Line, pos.Column)
					// fmt.Printf("\t\t-- Sel: %v\n", selection)
					// fmt.Printf("\t\t-- Kind: \t%v\n", selection.Kind())
					switch selection.Kind() {
					case types.FieldVal:
						fmt.Printf("\t\t-- Kind: \tField\n")
					case types.MethodVal:
						fmt.Printf("\t\t-- Kind: \tMethod\n")
					case types.MethodExpr:
						fmt.Printf("\t\t-- Kind: \tMethodExpr\n")
					}
					fmt.Printf("\t\t-- Recv: \t%v\n", selection.Recv().String())
					fmt.Printf("\t\t-- Indx: \t%d\n", selection.Index())
					fmt.Printf("\t\t-- Objc: \t%T\n", selection.Obj())
					if selection.Obj().Pos().IsValid() {
						var opos = fileSet.Position(selection.Obj().Pos())
						if len(opos.Filename) > 0 {
							fmt.Printf("\t\t-- OPos: \t%s:%d:%d\n", opos.Filename, opos.Line, opos.Column)
						}

					}
					fmt.Printf("\t\t-- Indr: \t%v\n", selection.Indirect())
				}
			*/
			/*
				fmt.Printf("\tScopes:\n")
				for node, scope := range info.Scopes {
					var pos = fileSet.Position(node.Pos())
					fmt.Printf("\t\t[%T]:\t%s:%d:%d\n", node, pos.Filename, pos.Line, pos.Column)
					for _, name := range scope.Names() {
						object := scope.Lookup(name)
						var opos = fileSet.Position(object.Pos())
						fmt.Printf("\t\t\t-- %s: \t%s\n", name, opos)
					}
				}
			*/
			/*
				fmt.Printf("\tE-Types:\n")
				for expr, typeVal := range info.Types {
					var epos = fileSet.Position(expr.Pos())
					fmt.Printf("\t\t%T: %s:%d:%d\n", expr, epos.Filename, epos.Line, epos.Column)
					if ident, ok := expr.(*ast.Ident); ok {
						fmt.Printf("\t\t\tNameRef:\t%s\n", ident.Name)
					}
					fmt.Printf("\t\t\tTypeRef:\t%v\n", typeVal.Type)
					if typeVal.IsVoid() {
						fmt.Printf("\t\t\t--- IsVoid: \t%v\n", typeVal.IsVoid())
					}
					if typeVal.IsType() {
						fmt.Printf("\t\t\t--- IsType: \t%v\n", typeVal.IsType())
					}
					if typeVal.IsValue() {
						fmt.Printf("\t\t\t--- IsValue:\t%v\n", typeVal.IsValue())
					}
					if typeVal.IsBuiltin() {
						fmt.Printf("\t\t\t--- IsBuilt:\t%v\n", typeVal.IsBuiltin())
					}
					if typeVal.HasOk() {
						fmt.Printf("\t\t\t--- HasOk:  \t%v\n", typeVal.HasOk())
					}
					if typeVal.Assignable() {
						fmt.Printf("\t\t\t--- Assign: \t%v\n", typeVal.Assignable())
					}
					if typeVal.Addressable() {
						fmt.Printf("\t\t\t--- Address:\t%v\n", typeVal.Addressable())
					}
				}
			*/
			/*
				fmt.Printf("\t\tMethods:\n")
				for expr, typeVal := range info.Types {
					if typeVal.Type != nil {
						var methods = types.NewMethodSet(typeVal.Type)
						if methods.Len() > 0 {
							var epos = fileSet.Position(expr.Pos())
							fmt.Printf("\t\t%T: %s:%d:%d\n", expr, epos.Filename, epos.Line, epos.Column)
							fmt.Printf("\t\tType:\t%s\n", typeVal.Type.String())
							for i := 0; i < methods.Len(); i++ {
								selection := methods.At(i)
								fmt.Printf("\t\tMethods[%d]\n", i)
								switch selection.Kind() {
								case types.FieldVal:
									fmt.Printf("\t\t-- Kind: \tField\n")
								case types.MethodVal:
									fmt.Printf("\t\t-- Kind: \tMethod\n")
								case types.MethodExpr:
									fmt.Printf("\t\t-- Kind: \tMethodExpr\n")
								}
								fmt.Printf("\t\t-- Recv: \t%v\n", selection.Recv().String())
								fmt.Printf("\t\t-- Indx: \t%d\n", selection.Index())
								fmt.Printf("\t\t-- Objc: \t%T\n", selection.Obj())
								switch obj := selection.Obj().(type) {
								case *types.Func:
									fmt.Printf("\t\t-- Func: \t%v\n", obj.Name())
								}
								fmt.Printf("\t\t-- Indr: \t%v\n", selection.Indirect())
							}
							fmt.Println()
						}
					}
				}
			*/
			fmt.Printf("\t\tSizes:\t%v\n", config.Sizes)
			for expr, typeVal := range info.Types {
				var epos = fileSet.Position(expr.Pos())
				fmt.Printf("\t\t%T: %s:%d:%d\n", expr, epos.Filename, epos.Line, epos.Column)
				fmt.Printf("\t\t\tTypeRef:\t%v\n", typeVal.Type)
				fmt.Printf("\t\t\tTypeSiz:\t%s bytes\n", sizeof(typeVal.Type, config.Sizes))
			}
		}
		fmt.Println()
	}
}

func sizeof(typ types.Type, sizes types.Sizes) (result string) {
	defer func() {
		if e := recover(); e != nil {
		}
	}()
	result = "?"
	if typ != nil && sizes != nil {
		return fmt.Sprintf("%d", sizes.Sizeof(typ))
	}
	return
}

func viewLoadConfigPackage(rootDir string) {
	// 1. read the go.mod file directly
	var goModPath = filepath.Join(rootDir, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		panic("go.mod not found: " + rootDir)
	}
	goModBytes, err := os.ReadFile(goModPath)
	if err != nil || len(goModBytes) == 0 {
		panic(fmt.Sprintf("unable to read: %s", goModPath))
	}

	// 2. fetch the dependency package's path
	var lines = strings.Split(string(goModBytes), "\n")
	var depPkgs []string
	var depPkgsToPaths = make(map[string]string) // pkg to dep path
	var moduleName string
	for _, line := range lines {
		if strings.HasPrefix(line, "module ") {
			moduleName = strings.Trim(line, "module ")
			depPkgs = append(depPkgs, moduleName)
		}
		if strings.HasPrefix(line, "\t") {
			items := strings.Split(strings.TrimSpace(line), " ")
			if len(items) >= 2 {
				depPkgPath := strings.TrimSpace(items[0])
				depPkgVersion := strings.TrimSpace(items[1])
				if len(depPkgPath) > 0 && len(depPkgVersion) > 0 {
					depPkgsToPaths[depPkgPath] = depPkgVersion
					depPkgs = append(depPkgs, depPkgPath)
				}
			}
		}
	}

	// 3. initialize the loader.Config
	fileSet := token.NewFileSet()
	// prefix := filepath.Join(build.Default.GOPATH, "pkg/mod")
	// fmt.Printf("\t--> %s\n", depPkgs)
	config := &packages.Config{
		Mode:       packages.LoadAllSyntax,
		Dir:        rootDir,
		BuildFlags: nil,
		Fset:       fileSet,
		ParseFile:  nil,
	}
	initPkgs, err := packages.Load(config, depPkgs...)
	if err != nil {
		fmt.Printf("\tERR-L: %v\n", initPkgs)
	}
	prog, pkgs := ssautil.Packages(initPkgs, ssa.SanityCheckFunctions)
	if prog == nil {
		panic(fmt.Sprintf("\tERR-SSA: %s", pkgs))
	}
	prog.Build()

	for _, pkg := range prog.AllPackages() {
		if pkg != nil {
			fmt.Printf("Pkg: %s\n", pkg.Pkg.Path())
			/*
				for name, member := range pkg.Members {
					fmt.Printf("\t%s:\t%T\n", name, member)
				}
			*/
			fmt.Println()
		} else {
			// fmt.Printf("Err: %d", i)
		}
	}

}

func viewLoadConfigAstType(rootDir string) {
	// 1. read the go.mod file directly
	var goModPath = filepath.Join(rootDir, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		panic("go.mod not found: " + rootDir)
	}
	goModBytes, err := os.ReadFile(goModPath)
	if err != nil || len(goModBytes) == 0 {
		panic(fmt.Sprintf("unable to read: %s", goModPath))
	}

	// 2. fetch the dependency package's path
	var lines = strings.Split(string(goModBytes), "\n")
	var depPkgs []string
	var depPkgsToPaths = make(map[string]string) // pkg to dep path
	var moduleName string
	for _, line := range lines {
		if strings.HasPrefix(line, "module ") {
			moduleName = strings.Trim(line, "module ")
			depPkgs = append(depPkgs, moduleName)
		}
		if strings.HasPrefix(line, "\t") {
			items := strings.Split(strings.TrimSpace(line), " ")
			if len(items) >= 2 {
				depPkgPath := strings.TrimSpace(items[0])
				depPkgVersion := strings.TrimSpace(items[1])
				if len(depPkgPath) > 0 && len(depPkgVersion) > 0 {
					depPkgsToPaths[depPkgPath] = depPkgVersion
					depPkgs = append(depPkgs, depPkgPath)
				}
			}
		}
	}

	// 3. initialize the loader.Config
	fileSet := token.NewFileSet()
	// prefix := filepath.Join(build.Default.GOPATH, "pkg/mod")
	// fmt.Printf("\t--> %s\n", depPkgs)
	config := &packages.Config{
		Mode:       packages.LoadAllSyntax,
		Dir:        rootDir,
		BuildFlags: nil,
		Fset:       fileSet,
		ParseFile:  nil,
	}
	pkgObjs, err := packages.Load(config, depPkgs...)
	if err != nil {
		panic(fmt.Sprintf("\tERR-L: %v\n", err))
	}

	// 4. print the initial packages
	for _, pkgObj := range pkgObjs {
		if pkgObj == nil {
			continue
		}

		fmt.Printf("Pkg: %s\t%s\n", pkgObj.Name, pkgObj.PkgPath)
		for _, file := range pkgObj.Syntax {
			var pos = pkgObj.Fset.Position(file.Pos())
			fmt.Printf("\t--- File:\t%s:%d:%d\n", pos.Filename, pos.Line, pos.Column)

			var typesNumber, errorNumber = 0, 0
			ast.Inspect(file, func(node ast.Node) bool {
				if node == nil {
					return false
				}
				expr, ok := node.(ast.Expr)
				if ok && isValidExpr(expr) {
					typ := pkgObj.TypesInfo.TypeOf(expr)
					if isValidType(typ) {
						typesNumber++
					} else {
						errorNumber++
					}
				}
				return true
			})
			fmt.Printf("\t\tPass: %d types, %d errors (%v).\n",
				typesNumber, errorNumber, percent(typesNumber, errorNumber))
		}

		fmt.Println()
	}

}

func testCompileForOneFile(rootDir string) {
	var pkgToSrcFiles = findPackagesAndGoFiles(rootDir)
	var passNumber, errorNumber = 0, 0
	for _, srcFiles := range pkgToSrcFiles {
		for _, srcFile := range srcFiles {
			var syntax, pkg, err = golang.LoadOneFile(srcFile)
			if err != nil || syntax == nil {
				fmt.Printf("\tERR-L:\t%v\n", err)
				errorNumber++
			} else {
				fmt.Printf("\tSRC-D:\t%s\n", srcFile)
				var typeNumber, noneNumber = 0, 0
				ast.Inspect(syntax, func(node ast.Node) bool {
					if node == nil {
						return false
					}
					expr, ok := node.(ast.Expr)
					if ok && isValidExpr(expr) {
						typ := pkg.TypesInfo.TypeOf(expr)
						if isValidType(typ) {
							typeNumber++
						} else {
							noneNumber++
						}
					}
					return true
				})
				fmt.Printf("\t\tPkg:  %s\n", pkg.PkgPath)
				var ratio = percent(typeNumber, noneNumber)
				var warning string
				if ratio < 70 {
					warning = "Warning!"
				}
				fmt.Printf("\t\tType: +%d, -%d (%v)\t%s\n", typeNumber,
					noneNumber, percent(typeNumber, noneNumber), warning)
				passNumber++
			}
		}

		/*
			loadPkg, err := analysis.LoadOnePkg(pkgDir)
			if err != nil {
				fmt.Printf("\tERR-L:\t%s\n", err.Error())
				errorNumber++
			} else if loadPkg == nil {
				fmt.Printf("\tERR-P:\t%s\n", pkgDir)
				errorNumber++
			} else {
				fmt.Printf("\tPKG-D:\t%s\n", pkgDir)
				fmt.Printf("\t\tFile: %d/%d (%v)\n", len(loadPkg.Syntax),
					len(srcFiles), len(loadPkg.Syntax) >= len(srcFiles))
				checkLoadPackageInfo(loadPkg)
				passNumber++
			}
		*/
	}
	fmt.Printf("\nTotal:\t%d pass & %d fail (%v).\n",
		passNumber, errorNumber, percent(passNumber, errorNumber))
}

func checkLoadPackageInfo(pkg *packages.Package) {
	var typeNumber, noneNumber int
	for _, syntax := range pkg.Syntax {
		ast.Inspect(syntax, func(node ast.Node) bool {
			if node == nil {
				return false
			}
			expr, ok := node.(ast.Expr)
			if ok && isValidExpr(expr) {
				typ := pkg.TypesInfo.TypeOf(expr)
				if isValidType(typ) {
					typeNumber++
				} else {
					noneNumber++
				}
			}
			return true
		})
	}
	fmt.Printf("\t\tPath: %s\n", pkg.PkgPath)
	fmt.Printf("\t\tType: +%d, -%d (%v)\n", typeNumber,
		noneNumber, percent(typeNumber, noneNumber))
}
