package golang

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
)

// readGoPackageIn reads the package name from go source file.
func readGoPackageIn(goFile string) (string, error) {
	// 1. read the content from the source code file
	if _, err := os.Stat(goFile); os.IsNotExist(err) {
		return "", err
	}
	var bytes, err = os.ReadFile(goFile)
	if err != nil {
		return "", err
	}

	// 2. find the package declaration from the file
	lines := strings.Split(string(bytes), NewLine)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, PackagePrefix) {
			return strings.TrimSpace(line[len(PackagePrefix):]), nil
		}
	}
	return "", fmt.Errorf("no package name is found")
}

// inferGoPkgInfo infers the package's path (pkgPath), reference name (pkgName),
// package directory path (pkgDir), or empty if error occurs (err is not a nil).
//
// The file can be either a go source file (.go) or directory, depending on the
// users call this function to infer package of source file or simply directory.
//
// The sequence of outcomes are "pkgPath, pkgName, pkgDir, and potential error".
func inferGoPkgInfo(module *Module, file string) (string, string, string, error) {
	// 1. check the existence of file and its type
	filePath, _ := filepath.Abs(file)
	fileInfo, err := os.Stat(filePath)
	if module == nil {
		return "", "", "", fmt.Errorf("no go module info is provided")
	} else if os.IsNotExist(err) {
		return "", "", "", fmt.Errorf("file not exist: %s", filePath)
	} else if fileInfo == nil {
		return "", "", "", fmt.Errorf("can't get info: %s", filePath)
	}

	// 2. infer the package path, name and file path of directory
	if fileInfo.IsDir() {
		relPath, err := filepath.Rel(module.RootPath, filePath)
		if err != nil {
			return "", "", "", err
		}
		pkgPath := filepath.Join(module.ModuleName, relPath)
		return pkgPath, filepath.Base(filePath), filePath, nil
	}

	// 3. infer the package path, name and file path of code file
	if strings.HasSuffix(filePath, GoFileSuffix) {
		pkgDir := filepath.Dir(filePath)
		relPath, err := filepath.Rel(module.RootPath, pkgDir)
		if err != nil {
			return "", "", "", err
		}
		pkgPath := filepath.Join(module.ModuleName, relPath)
		pkgName, err := readGoPackageIn(filePath)
		if err != nil {
			return "", "", "", err
		}
		return pkgPath, pkgName, pkgDir, nil
	}

	// 4. infer the package path, name and file path of other file
	pkgDir := filepath.Dir(filePath)
	relPath, err := filepath.Rel(module.RootPath, pkgDir)
	if err != nil {
		return "", "", "", err
	}
	pkgPath := filepath.Join(module.ModuleName, relPath)
	return pkgPath, filepath.Base(pkgDir), pkgDir, nil
}

// newDefaultTypeConfig returns types.Config in default template.
func newDefaultTypeConfig() *types.Config {
	return &types.Config{
		Context:                  types.NewContext(),
		IgnoreFuncBodies:         false,
		FakeImportC:              false,
		Error:                    func(err error) { /* do nothing */ },
		Importer:                 importer.Default(), // GOROOT types
		Sizes:                    types.SizesFor("gc", build.Default.GOARCH),
		DisableUnusedImportCheck: false,
	}
}

// newDefaultTypeInfo returns types.Info in the default template.
func newDefaultTypeInfo() *types.Info {
	return &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Instances:  make(map[*ast.Ident]types.Instance),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Implicits:  make(map[ast.Node]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Scopes:     make(map[ast.Node]*types.Scope),
		InitOrder:  nil,
	}
}

// parseSourceFileByFree freely builds the source file using syntax parser and
// a basic type checking mode.
func parseSourceFileByFree(srcFile *SrcFile) error {
	// 1. read the source code
	if srcFile == nil || srcFile.Package() == nil {
		return fmt.Errorf("incomplete: %s", srcFile.Path())
	}
	var srcBytes, readErr = os.ReadFile(srcFile.Path())
	if readErr != nil {
		return readErr
	}
	if len(srcBytes) == 0 {
		return fmt.Errorf("empty file: %s", srcFile.Path())
	}

	// 2. parse the syntax
	var fileSet = token.NewFileSet()
	var syntax, parseErr = parser.ParseFile(
		fileSet, srcFile.Path(), nil, parser.ParseComments)
	if parseErr != nil {
		return parseErr
	}
	if syntax == nil {
		return fmt.Errorf("can't parse: %s", srcFile.Path())
	}
	_ = srcFile.update(string(srcBytes), syntax, nil)

	// 3. perform default type checking
	typeConf := newDefaultTypeConfig()
	typeInfo := newDefaultTypeInfo()
	typePkg, typeErr := typeConf.Check(srcFile.Package().PkgPath(), fileSet, []*ast.File{syntax}, typeInfo)
	if typePkg == nil {
		return fmt.Errorf("can't create types.Package: %s", srcFile.Package().PkgPath())
	}
	pkg := srcFile.Package()

	// 4. update the package object
	pkg.fileSet = fileSet
	pkg.typePkg = typePkg
	pkg.typInfo = typeInfo
	pkg.typSize = &typeConf.Sizes
	for _, importSpec := range syntax.Imports {
		if importSpec != nil && importSpec.Path != nil {
			importPath := strings.Trim(importSpec.Path.Value, "\"")
			pkg.imports = append(pkg.imports, importPath)
		}
	}

	// 5. record the current load info
	pkg.loadInfo = &LoadInfo{
		LoadTime:     time.Now(),
		LoadedFiles:  []string{srcFile.Path()},
		IgnoredFiles: nil,
		IllTyped:     typeErr != nil,
		FileErrors:   nil,
		TypeErrors:   nil,
		DepsErrors:   nil,
	}
	if typeErr != nil {
		pkg.loadInfo.TypeErrors = []error{typeErr}
	}
	return nil
}

// loadSourceFileByFree 'freely' loads the source file in the given path, then
// return the SrcFile object (along with its Package and Program), if possible.
//
// If no 'go.mod' is found in the parent directories of source file, then this
// function returns a SrcFile, with only the Package from the parent directory.
func loadSourceFileByFree(codeFile string) (*SrcFile, error) {
	// 1. validate the input go source file
	codePath, _ := filepath.Abs(codeFile)
	fileInfo, err := os.Stat(codePath)
	if os.IsNotExist(err) {
		return nil, err
	} else if fileInfo.IsDir() {
		return nil, fmt.Errorf("not go file: %s", codePath)
	} else if !strings.HasSuffix(codePath, GoFileSuffix) {
		return nil, fmt.Errorf(" not go file: %s", codePath)
	}

	// 2. infer package path, name and dir
	program, _ := initProgram(filepath.Dir(codePath))
	if program != nil && program.module != nil {
		pkgPath, pkgName, pkgDir, err := inferGoPkgInfo(program.module, codePath)
		if err != nil {
			return nil, fmt.Errorf("can't get package: %v", err.Error())
		}
		pkg := program.newPackage(pkgName, pkgPath, pkgDir)
		if pkg == nil {
			return nil, fmt.Errorf("can't new package: %s", pkgPath)
		}
		srcFile := pkg.newSrcFile(codePath)
		if srcFile == nil {
			return nil, fmt.Errorf("can't new source file: %s", codePath)
		}
		parseErr := parseSourceFileByFree(srcFile)
		if parseErr != nil {
			return nil, parseErr
		}
		return srcFile, nil
	}

	// 3. infer a package from absolute path
	pkgDir := filepath.Dir(codePath)
	pkgName, err := readGoPackageIn(codePath)
	if err != nil {
		return nil, err
	}
	pkg := newPackage(nil, pkgName, pkgDir, pkgDir)
	if pkg == nil {
		return nil, fmt.Errorf("can't new package: %s", pkgDir)
	}
	srcFile := pkg.newSrcFile(codePath)
	if srcFile == nil {
		return nil, fmt.Errorf("can't new source file: %s", codePath)
	}
	parseErr := parseSourceFileByFree(srcFile)
	if parseErr != nil {
		return nil, parseErr
	}
	return srcFile, nil
}

// parseGoPackageByFree freely parses the package with the info of syntax pkg.
// It returns the load error if parsing failed.
func parseGoPackageByFree(pkg *Package, astPkg *ast.Package) error {
	// 1. initialize the loading info
	if pkg == nil || astPkg == nil || len(astPkg.Files) == 0 {
		return fmt.Errorf("no go files in: %v", pkg)
	}
	loadInfo := &LoadInfo{LoadTime: time.Now()}
	pkg.loadInfo = loadInfo

	// 2. construct each source file in package
	var astFiles []*ast.File
	for _, syntax := range astPkg.Files {
		if syntax == nil {
			continue
		}
		var srcPath = pkg.fileSet.Position(syntax.Pos()).Filename
		srcPath, _ = filepath.Abs(srcPath)
		var bytes, readErr = os.ReadFile(srcPath)
		if readErr != nil {
			loadInfo.FileErrors = append(loadInfo.FileErrors, readErr)
			continue
		} else if len(bytes) == 0 {
			loadInfo.FileErrors = append(loadInfo.FileErrors,
				fmt.Errorf("empty file: %s", srcPath))
			continue
		}
		var srcFile = pkg.newSrcFile(srcPath)
		_ = srcFile.update(string(bytes), syntax, nil)
		astFiles = append(astFiles, syntax)
		loadInfo.LoadedFiles = append(loadInfo.LoadedFiles, srcPath)
	}

	// 3. perform the type checking
	typeConf := newDefaultTypeConfig()
	typeInfo := newDefaultTypeInfo()
	typePkg, typeErr := typeConf.Check(pkg.PkgPath(), pkg.FileSet(), astFiles, typeInfo)
	if typeErr != nil {
		loadInfo.IllTyped = true
		loadInfo.TypeErrors = append(loadInfo.TypeErrors, typeErr)
	} else if typePkg == nil {
		loadInfo.IllTyped = true
		loadInfo.TypeErrors = append(
			loadInfo.TypeErrors, fmt.Errorf("no types.Package"))
	}
	pkg.typePkg = typePkg
	pkg.typInfo = typeInfo
	pkg.typSize = &typeConf.Sizes

	// 4. update the imported paths in the source files
	var imports = make(map[string]bool)
	for _, syntax := range astPkg.Files {
		if syntax == nil {
			continue
		}
		for _, importSpec := range syntax.Imports {
			if importSpec == nil || importSpec.Path == nil {
				continue
			}
			importPath := importSpec.Path.Value
			importPath = strings.Trim(importPath, "\"")
			if len(importPath) > 0 {
				imports[importPath] = true
			}
		}
	}
	for importPath, _ := range imports {
		pkg.imports = append(pkg.imports, importPath)
	}

	return nil // complete all finally
}

// loadGoDirectoryByFree 'freely' loads the source files in this go directory,
// not including those in its recursive children.
func loadGoDirectoryByFree(goDir string) ([]*Package, error) {
	// 1. validate the input directory
	goDirPath, _ := filepath.Abs(goDir)
	fileInfo, err := os.Stat(goDirPath)
	if os.IsNotExist(err) {
		return nil, err
	}
	if !fileInfo.IsDir() {
		return nil, fmt.Errorf("not directory: %s", goDirPath)
	}

	// 2. parse the source files in dir
	fileSet := token.NewFileSet()
	pkgs, parseErr := parser.
		ParseDir(fileSet, goDirPath, nil, parser.ParseComments)
	if parseErr != nil {
		return nil, parseErr
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no go files in: %s", goDirPath)
	}

	// 3. get the program and module info
	var newPackages []*Package
	program, modErr := initProgram(goDirPath)
	if modErr == nil && program != nil && program.module != nil {
		pkgPath, pkgName, _, findErr := inferGoPkgInfo(program.module, goDirPath)
		if findErr != nil {
			return nil, fmt.Errorf("can't infer package path: %s", goDirPath)
		}
		for pkgKey, astPkg := range pkgs {
			if len(pkgKey) > 0 && astPkg != nil && len(astPkg.Files) > 0 {
				newPkgPath := pkgPath
				if pkgKey != pkgName {
					newPkgPath = fmt.Sprintf("%s/%s", pkgPath, pkgKey)
				}
				pkg := program.newPackage(pkgKey, newPkgPath, goDirPath)
				if pkg != nil {
					pkg.fileSet = fileSet
					loadErr := parseGoPackageByFree(pkg, astPkg)
					if loadErr == nil {
						newPackages = append(newPackages, pkg)
					}
				}
			}
		}
		return newPackages, nil
	}

	// 4. cannot find go mod
	return nil, fmt.Errorf("can't find go.mod in: %s", goDirPath)
}

// loadAllDirectoriesByFree freely load the source files and their packages in
// the root-directory as given. A 'go.mod' is required in rootDir or any of its
// parent directories, or none is returned.
func loadAllDirectoriesByFree(rootDir string) ([]*Package, error) {
	// 1. validate the input directory
	rootDirPath, _ := filepath.Abs(rootDir)
	fileInfo, err := os.Stat(rootDirPath)
	if os.IsNotExist(err) {
		return nil, err
	}
	if !fileInfo.IsDir() {
		return nil, fmt.Errorf("not directory: %s", rootDirPath)
	}

	// 2. get the go.mod and module info
	fileSet := token.NewFileSet()
	program, modErr := initProgram(rootDirPath)
	if modErr != nil {
		return nil, modErr
	}
	if program == nil || program.module == nil {
		return nil, fmt.Errorf("no go.mod is found: %s", rootDir)
	}

	// 3. construct the mapping from Package to ast.Package for parsing
	var newPackages []*Package
	for pkgDir, goFiles := range findPackagesAndGoFiles(rootDirPath) {
		if len(pkgDir) == 0 || len(goFiles) == 0 {
			continue
		}

		astPkgs, parseErr := parser.ParseDir(fileSet, pkgDir, nil, parser.ParseComments)
		if parseErr != nil || astPkgs == nil || len(astPkgs) == 0 {
			continue
		}

		pkgPath, pkgName, _, pkgErr := inferGoPkgInfo(program.module, pkgDir)
		if pkgErr != nil {
			continue
		}

		for pkgKey, astPkg := range astPkgs {
			if len(pkgKey) > 0 && astPkg != nil && len(astPkg.Files) > 0 {
				newPkgPath := pkgPath
				if pkgKey != pkgName {
					newPkgPath = fmt.Sprintf("%s/%s", pkgPath, pkgKey)
				}
				pkg := program.newPackage(pkgKey, newPkgPath, pkgDir)
				if pkg != nil {
					pkg.fileSet = fileSet
					loadErr := parseGoPackageByFree(pkg, astPkg)
					if loadErr == nil {
						newPackages = append(newPackages, pkg)
					}
				}
			}
		}
	}
	return newPackages, nil
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
