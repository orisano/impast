package impast

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/tools/go/packages"
)

func ignoreTestFile(info os.FileInfo) bool {
	return !strings.HasSuffix(info.Name(), "_test.go")
}

func ImportPackage(importPath string) (*ast.Package, error) {
	pkgs, err := packages.Load(&packages.Config{}, importPath)
	if err != nil {
		return nil, errors.Wrapf(err, "impast: failed to load package: %q", importPath)
	}
	if len(pkgs) != 1 {
		return nil, errors.Errorf("impast: invalid import path: %q", importPath)
	}

	pkgPath := filepath.Dir(pkgs[0].GoFiles[0])
	fset := token.NewFileSet()
	astPkgs, err := parser.ParseDir(fset, pkgPath, ignoreTestFile, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "impast: broken package %q", pkgPath)
	}
	if len(pkgs) > 1 {
		delete(astPkgs, "main")
	}
	if len(pkgs) != 1 {
		return nil, errors.Errorf("impast: ambiguous packages, found %d packages", len(pkgs))
	}
	for _, pkg := range astPkgs {
		return pkg, nil
	}
	return nil, errors.Errorf("impast: package not found %q", importPath)
}

func ScanDecl(pkg *ast.Package, f func(ast.Decl) bool) {
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			if !f(decl) {
				return
			}
		}
	}
}

func ExportType(pkg *ast.Package, expr ast.Expr) ast.Expr {
	switch expr := expr.(type) {
	case *ast.Ident:
		if !expr.IsExported() {
			return expr
		}
		return &ast.SelectorExpr{Sel: expr, X: ast.NewIdent(pkg.Name)}
	case *ast.StarExpr:
		return &ast.StarExpr{X: ExportType(pkg, expr.X)}
	case *ast.ArrayType:
		return &ast.ArrayType{Elt: ExportType(pkg, expr.Elt), Len: expr.Len}
	case *ast.MapType:
		return &ast.MapType{Key: ExportType(pkg, expr.Key), Value: ExportType(pkg, expr.Value)}
	case *ast.ChanType:
		return &ast.ChanType{Begin: expr.Begin, Arrow: expr.Arrow, Dir: expr.Dir, Value: ExportType(pkg, expr.Value)}
	case *ast.FuncType:
		fn := *expr
		fn.Params = ExportFields(pkg, fn.Params)
		fn.Results = ExportFields(pkg, fn.Results)
		return &fn
	case *ast.InterfaceType:
		it := *expr
		it.Methods = ExportFields(pkg, it.Methods)
		return &it
	case *ast.Ellipsis:
		return &ast.Ellipsis{Ellipsis: expr.Ellipsis, Elt: ExportType(pkg, expr.Elt)}
	default:
		return expr
	}
}

func ExportFields(pkg *ast.Package, fields *ast.FieldList) *ast.FieldList {
	if fields == nil {
		return nil
	}
	efields := *fields
	for i, field := range efields.List {
		efields.List[i].Type = ExportType(pkg, field.Type)
	}
	return &efields
}

func ExportFunc(pkg *ast.Package, fn *ast.FuncDecl) *ast.FuncDecl {
	efn := *fn
	efn.Recv = nil
	efn.Type = ExportType(pkg, efn.Type).(*ast.FuncType)
	return &efn
}

func GetMethods(pkg *ast.Package, name string) []*ast.FuncDecl {
	var methods []*ast.FuncDecl
	ScanDecl(pkg, func(decl ast.Decl) bool {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			return true
		}
		if funcDecl.Recv == nil {
			return true
		}
		rt := funcDecl.Recv.List[0]
		if TypeName(rt.Type) == name && funcDecl.Name.IsExported() {
			methods = append(methods, funcDecl)
		}
		return true
	})
	return methods
}

func FindTypeByName(pkg *ast.Package, name string) ast.Expr {
	var t ast.Expr
	ScanDecl(pkg, func(decl ast.Decl) bool {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			return true
		}
		for _, spec := range genDecl.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			if typeSpec.Name.Name == name {
				t = typeSpec.Type
				return false
			}
		}
		return true
	})
	return t
}

func FindInterface(pkg *ast.Package, name string) *ast.InterfaceType {
	it, ok := FindTypeByName(pkg, name).(*ast.InterfaceType)
	if !ok {
		return nil
	}
	return it
}

func FindStruct(pkg *ast.Package, name string) *ast.StructType {
	st, ok := FindTypeByName(pkg, name).(*ast.StructType)
	if !ok {
		return nil
	}
	return st
}

func TypeName(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var b bytes.Buffer
	if err := printer.Fprint(&b, token.NewFileSet(), expr); err != nil {
		panic(err)
	}
	return b.String()
}

func GetRequires(it *ast.InterfaceType) []*ast.Field {
	var fields []*ast.Field
	has := map[string]struct{}{}

	add := func(f *ast.Field) {
		name := f.Names[0].Name
		if _, ok := has[name]; ok {
			return
		}
		has[name] = struct{}{}
		fields = append(fields, f)
	}

	for _, field := range it.Methods.List {
		if len(field.Names) == 0 {
			for _, f := range GetRequires(field.Type.(*ast.Ident).Obj.Decl.(*ast.TypeSpec).Type.(*ast.InterfaceType)) {
				add(f)
			}
		} else {
			add(field)
		}
	}
	return fields
}

func AutoNaming(ft *ast.FuncType) *ast.FuncType {
	t := *ft
	if len(t.Params.List) == 0 {
		return &t
	}
	if len(t.Params.List[0].Names) != 0 {
		return &t
	}
	for i := range t.Params.List {
		t.Params.List[i].Names = append(t.Params.List[i].Names, ast.NewIdent(fmt.Sprintf("arg%d", i+1)))
	}
	return &t
}

}
