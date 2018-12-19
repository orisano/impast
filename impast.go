package impast

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

var (
	PackageNotFound = errors.New("package not found")
	TypeNotFound    = errors.New("type not found")
)

func ignoreTestFile(info os.FileInfo) bool {
	return !strings.HasSuffix(info.Name(), "_test.go")
}

type Importer struct {
	EnableCache bool
	cache       sync.Map
}

var DefaultImporter Importer

func (i *Importer) Load(pkgs map[string]*ast.Package) {
	for p, pkg := range pkgs {
		i.cache.Store(p, pkg)
	}
}

func (i *Importer) Loaded() []string {
	var paths []string
	i.cache.Range(func(key, _ interface{}) bool {
		paths = append(paths, key.(string))
		return true
	})
	return paths
}

func (i *Importer) ImportPackage(importPath string) (*ast.Package, error) {
	if i.EnableCache {
		if v, ok := i.cache.Load(importPath); ok {
			return v.(*ast.Package), nil
		}
	}
	pkg, err := build.Import(importPath, ".", build.FindOnly)
	if err != nil {
		return nil, errors.Wrap(err, "failed to import")
	}

	pkgPath := pkg.Dir
	fset := token.NewFileSet()
	astPkgs, err := parser.ParseDir(fset, pkgPath, ignoreTestFile, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "broken package %q", pkgPath)
	}
	if len(astPkgs) > 1 {
		delete(astPkgs, "main")
	}
	if len(astPkgs) != 1 {
		return nil, errors.Errorf("ambiguous packages, found %d packages", len(astPkgs))
	}
	for _, pkg := range astPkgs {
		if i.EnableCache {
			i.cache.Store(importPath, pkg)
		}
		return pkg, nil
	}
	return nil, errors.Errorf("package not found")
}

func ImportPackage(importPath string) (*ast.Package, error) {
	return DefaultImporter.ImportPackage(importPath)
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
	return DefaultImporter.GetMethodsDeep(pkg, name)
}

func (i *Importer) GetMethodsDeep(pkg *ast.Package, name string) ([]*ast.FuncDecl, error) {
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
				if err := i.resolveMethodsDeep(pkg, f, t, m); err != nil {
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

func (i *Importer) getEmbeddedMethods(pkg *ast.Package, f *ast.File, t ast.Expr) ([]*ast.FuncDecl, error) {
	p, name, err := i.ResolveType(f, t)
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve type")
	}
	if p == nil {
		p = pkg
	}
	return i.GetMethodsDeep(p, name)
}

func (i *Importer) resolveMethodsDeep(pkg *ast.Package, f *ast.File, t *ast.StructType, dest map[string]*ast.FuncDecl) error {
	for _, et := range getEmbeddedStruct(t) {
		methods, err := i.getEmbeddedMethods(pkg, f, et)
		if err != nil {
			return errors.Wrapf(err, "failed to get embedded methods: %v", TypeName(et))
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
	return DefaultImporter.ResolveType(f, expr)
}

func (i *Importer) ResolveType(f *ast.File, expr ast.Expr) (*ast.Package, string, error) {
	var pkg *ast.Package
	var err error
	if se, ok := expr.(*ast.StarExpr); ok {
		expr = se.X
	}
	if se, ok := expr.(*ast.SelectorExpr); ok {
		expr = se.Sel

		pkgName := se.X.(*ast.Ident).Name
		pkg, err = i.ResolvePackage(f, pkgName)
		if err != nil {
			return nil, "", errors.Wrapf(err, "failed to resolve package: %v", se.Sel.Name)
		}
	} else if id, ok := expr.(*ast.Ident); ok && id.IsExported() {
		pkg, err = i.ImportPackage(".")
		wd, _ := os.Getwd()
		if err != nil {
			return nil, "", errors.Wrapf(err, "failed to import self package: %v", wd)
		}
	}
	name := expr.(*ast.Ident).Name
	return pkg, name, nil
}

func ResolvePackage(f *ast.File, name string) (*ast.Package, error) {
	return DefaultImporter.ResolvePackage(f, name)
}

func (i *Importer) ResolvePackage(f *ast.File, name string) (*ast.Package, error) {
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid import path (%v)", imp.Path.Value)
		}

		if imp.Name == nil || imp.Name.Name == name {
			pkg, err := i.ImportPackage(p)
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
