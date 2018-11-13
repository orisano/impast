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
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/tools/go/packages"
)

var (
	PackageNotFound = errors.New("package not found")
	TypeNotFound    = errors.New("type not found")
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

func GetMethodsDeep(pkg *ast.Package, name string) ([]*ast.FuncDecl, error) {
	return GetMethodsDeepWithCache(pkg, name, map[string]*ast.Package{})
}

func GetMethodsDeepWithCache(pkg *ast.Package, name string, pkgs map[string]*ast.Package) ([]*ast.FuncDecl, error) {
	var t *ast.StructType

	m := map[string]*ast.FuncDecl{}
	for _, f := range pkg.Files {
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if isOwnMethod(name, d) && d.Name.IsExported() {
					m[d.Name.Name] = d
				}
			case *ast.GenDecl:
				if t != nil {
					continue
				}
				st, found, err := findStruct(d, name)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to find struct: %+v", d)
				}
				if !found {
					continue
				}
				t = st
				if err := resolveMethodsDeep(pkg, f, t, pkgs, m); err != nil {
					return nil, errors.Wrap(err, "failed to resolve methods")
				}
			}
		}
	}
	if t == nil {
		return nil, TypeNotFound
	}
	methods := make([]*ast.FuncDecl, 0, len(m))
	for _, f := range m {
		methods = append(methods, ExportFunc(pkg, f))
	}
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Name.Name < methods[j].Name.Name
	})
	return methods, nil
}

func findStruct(d *ast.GenDecl, name string) (*ast.StructType, bool, error) {
	if d.Tok != token.TYPE {
		return nil, false, nil
	}
	for _, spec := range d.Specs {
		typeSpec := spec.(*ast.TypeSpec)
		if typeSpec.Name.Name != name {
			continue
		}
		st, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return nil, false, errors.Errorf("is not struct: %+v", typeSpec.Type)
		}
		return st, true, nil
	}
	return nil, false, nil
}

func getEmbeddedStruct(s *ast.StructType) []ast.Expr {
	var es []ast.Expr
	for _, f := range s.Fields.List {
		if len(f.Names) > 0 {
			continue
		}
		es = append(es, f.Type)
	}
	return es
}

func getEmbeddedMethods(pkg *ast.Package, f *ast.File, t ast.Expr, pkgs map[string]*ast.Package) ([]*ast.FuncDecl, error) {
	p, name, err := ResolveTypeWithCache(f, t, pkgs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve type")
	}
	if p == nil {
		p = pkg
	}
	return GetMethodsDeepWithCache(p, name, pkgs)
}

func resolveMethodsDeep(pkg *ast.Package, f *ast.File, t *ast.StructType, pkgs map[string]*ast.Package, dest map[string]*ast.FuncDecl) error {
	for _, et := range getEmbeddedStruct(t) {
		methods, err := getEmbeddedMethods(pkg, f, et, pkgs)
		if err != nil {
			return errors.Wrapf(err, "failed to embedded methods: %v", TypeName(et))
		}
		for _, method := range methods {
			if _, ok := dest[method.Name.Name]; !ok {
				dest[method.Name.Name] = method
			}
		}
	}
	return nil
}

func ResolveType(f *ast.File, expr ast.Expr) (*ast.Package, string, error) {
	return ResolveTypeWithCache(f, expr, map[string]*ast.Package{})
}

func ResolveTypeWithCache(f *ast.File, expr ast.Expr, pkgs map[string]*ast.Package) (*ast.Package, string, error) {
	var pkg *ast.Package
	if se, ok := expr.(*ast.StarExpr); ok {
		expr = se.X
	}
	if se, ok := expr.(*ast.SelectorExpr); ok {
		expr = se.Sel

		pkgName := se.X.(*ast.Ident).Name
		var err error
		pkg, err = ResolvePackageWithCache(f, pkgName, pkgs)
		if err != nil {
			return nil, "", errors.Wrapf(err, "failed to resolve package: %v", se.Sel.Name)
		}
	}
	name := expr.(*ast.Ident).Name
	return pkg, name, nil
}

func ResolvePackage(f *ast.File, name string) (*ast.Package, error) {
	return ResolvePackageWithCache(f, name, map[string]*ast.Package{})
}

func ResolvePackageWithCache(f *ast.File, name string, pkgs map[string]*ast.Package) (*ast.Package, error) {
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid import path (%v)", imp.Path.Value)
		}

		if imp.Name == nil || imp.Name.Name == name {
			pkg, err := importPackageWithCache(p, pkgs)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to import (%v)", p)
			}
			if imp.Name != nil || pkg.Name == name {
				return pkg, nil
			}
		}
	}
	return nil, PackageNotFound
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
	printer.Fprint(&b, token.NewFileSet(), expr)
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

func importPackageWithCache(importPath string, pkgs map[string]*ast.Package) (*ast.Package, error) {
	if pkg, ok := pkgs[importPath]; ok {
		return pkg, nil
	}
	p, err := ImportPackage(importPath)
	if err != nil {
		return nil, err
	}
	pkgs[importPath] = p
	return p, nil
}

func isOwnMethod(name string, funcDecl *ast.FuncDecl) bool {
	if funcDecl.Recv == nil {
		return false
	}
	rt := funcDecl.Recv.List[0].Type
	if se, ok := rt.(*ast.StarExpr); ok {
		rt = se.X
	}
	return rt.(*ast.Ident).Name == name
}
