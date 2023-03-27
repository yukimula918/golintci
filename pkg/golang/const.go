package golang

import (
	"golang.org/x/tools/go/packages"
	"os"
)

const (
	NoneString     = ""
	GoFileSuffix   = ".go"
	GoModFileName  = "go.mod"
	PathSeparator  = "/"
	GoModulePrefix = "module "
	LoadFile       = packages.NeedName | packages.NeedFiles | packages.NeedSyntax
	LoadType       = packages.NeedTypes | packages.NeedTypesInfo | packages.NeedTypesSizes
	LoadDeps       = packages.NeedImports | packages.NeedDeps | packages.NeedModule
)

func newline() string {
	if os.PathSeparator == '/' {
		return "\n"
	} else {
		return "\r\n"
	}
}
