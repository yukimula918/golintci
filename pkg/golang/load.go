package golang

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// findGoModFileOf returns the absolute path of go.mod file that defines the
// module to which the directory belongs.
//
// If no go.mod file is found in the directory or its recursive parent, then
// the function returns an empty string (i.e. NoneString) as the output.
func findGoModFileOf(dirPath string) string {
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return NoneString
	}
	dirPath, _ = filepath.Abs(dirPath)
	for len(dirPath) > 0 && dirPath != "/" && dirPath != "." && dirPath != ".." {
		var goModInDir = filepath.Join(dirPath, GoModFileName)
		if _, err := os.Stat(goModInDir); !os.IsNotExist(err) {
			return goModInDir
		}
		parentDir := filepath.Dir(dirPath)
		if parentDir == dirPath {
			break
		}
		dirPath = parentDir
	}
	return NoneString
}

// findPackagePath returns the 'relative' path of the directory to the root where
// go.mod file exists, and returns the package path if the directory is a package.
//
// It returns the absolute path of directory if no go.mod file is found in parent.
func findPackagePath(dirPath string) string {
	// 1. get the go.mod or return dirPath simply
	dirPath, _ = filepath.Abs(dirPath)
	var goModFile = findGoModFileOf(dirPath)
	if len(goModFile) == 0 {
		return dirPath
	}

	// 2. infer the relative path of dir in root
	var rootPath = filepath.Dir(goModFile)
	var relPath, _ = filepath.Rel(rootPath, dirPath)
	relPath = strings.Trim(relPath, PathSeparator)

	// 3. get the module name from 'go.mod' file
	var bytes, err = os.ReadFile(goModFile)
	if err != nil {
		return relPath
	}
	var moduleName string
	for _, line := range strings.Split(string(bytes), newline()) {
		if strings.HasPrefix(line, GoModulePrefix) {
			moduleName = strings.Trim(line, GoModulePrefix)
			break
		}
	}

	// 4. connect the module with relative path
	if len(moduleName) > 0 {
		return fmt.Sprintf("%s/%s", moduleName, relPath)
	}
	return relPath
}
