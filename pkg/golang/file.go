// Package golang implements the model to load and represent syntax and semantic information from
// source code in the .go files.
//
// Specifically, this file defines SrcFile which provides interface to access the syntactic, type
// and semantic information from source code of one single file.
package golang

import (
	"fmt"
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// SrcFile represents a source code file in go program (ending with '.go' suffix).
//
// It is the smallest unit for analyzer, and implements interfaces to access code text, syntactic
// tree along with the static single assignment IR members loaded from this source during parsing.
//
// The syntax and semantic info of SrcFile can be updated by invoking SrcFile.update, which is an
// internal method that will (only) be used by Package when loading its source files from outside.
type SrcFile struct {
	pkg    *Package     // pkg refers to the Package in which this source file is contained
	path   string       // path is the absolute path of the source file that it represents
	code   string       // code is the text in the source file being analyzed
	syntax *ast.File    // syntax is the abstract syntax tree of source file (AST)
	memSet []ssa.Member // memSet are the static single assignment (SSA) members in the file
}

// newSrcFile is an internal method that ONLY be invoked by Package
func newSrcFile(pkg *Package, path string) *SrcFile {
	return &SrcFile{
		pkg:    pkg,
		path:   path,
		code:   NoneString,
		syntax: nil,
		memSet: nil,
	}
}

// Package refers to the Package in which this source file is contained
func (file *SrcFile) Package() *Package {
	if file != nil {
		return file.pkg
	}
	return nil
}

// Path is the absolute path of the source file that it represents
func (file *SrcFile) Path() string {
	if file != nil {
		return file.path
	}
	return NoneString
}

// Code is the text in the source file being analyzed
func (file *SrcFile) Code() string {
	if file != nil {
		return file.code
	}
	return NoneString
}

// Syntax is the abstract syntax tree of source file (AST) or nil if it has not been loaded
func (file *SrcFile) Syntax() *ast.File {
	if file != nil {
		return file.syntax
	}
	return nil
}

// Members return the static single assignment (SSA) members in the file or none if SSA form is not loaded.
func (file *SrcFile) Members() []ssa.Member {
	if file != nil {
		return file.memSet
	}
	return nil
}

// Contain checks whether the position is included by this source file.
func (file *SrcFile) Contain(pos token.Pos) bool {
	if file != nil && pos.IsValid() {
		path := file.pkg.fileSet.Position(pos).Filename
		return path == file.path
	}
	return false
}

// update will reset the syntax, type and semantic information of the source file.
func (file *SrcFile) update(code string, syntax *ast.File, members map[string]ssa.Member) error {
	if file != nil {
		file.code = code
		file.syntax = syntax
		file.memSet = nil
		if members != nil && len(members) > 0 {
			for _, member := range members {
				if member == nil {
					continue
				}
				if file.Contain(member.Pos()) {
					file.memSet = append(file.memSet, member)
				}
			}
		}
		return nil
	}
	return fmt.Errorf("nil file is used")
}
