package impast_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/orisano/impast"
)

func mustParseFile(src string) *ast.File {
	f, err := parser.ParseFile(token.NewFileSet(), "", src, 0)
	if err != nil {
		panic(err)
	}
	return f
}

func mustParseExpr(src string) ast.Expr {
	expr, err := parser.ParseExpr(src)
	if err != nil {
		panic(err)
	}
	return expr
}

func ExampleImportPackage() {
	pkg, err := impast.ImportPackage("io")
	if err != nil {
		log.Fatal(err)
	}
	it := impast.FindInterface(pkg, "Writer")
	if it == nil {
		log.Fatalf("io.Writer not found")
	}

	methods := impast.GetRequires(it)
	for _, method := range methods {
		fmt.Println(method.Names[0].Name)
	}
	// Output:
	// Write
}

func TestScanDecl(t *testing.T) {
	tests := []struct {
		pkg      *ast.Package
		stop     func(ast.Decl) bool
		expected int
	}{
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"sample.go": mustParseFile(`
package main

type Foo int
`),
				},
			},
			stop:     func(decl ast.Decl) bool { return false },
			expected: 1,
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"sample.go": mustParseFile(`
package main

type Foo int
type Bar int
`),
				},
			},
			stop:     func(decl ast.Decl) bool { return false },
			expected: 2,
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"sample.go": mustParseFile(`
package main

type (
	Foo int
	Bar int
)
type FooBar int
`),
				},
			},
			stop:     func(decl ast.Decl) bool { return false },
			expected: 2,
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"sample.go": mustParseFile(`
package main

type Foo int 
`),
					"main.go": mustParseFile(`
package main

type Bar int
`),
				},
			},
			stop:     func(decl ast.Decl) bool { return false },
			expected: 2,
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"sample.go": mustParseFile(`
package main

type Foo int
type Bar int
type FooBar int
type BarFoo int
`),
				},
			},
			stop: func(decl ast.Decl) bool {
				return decl.(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Name.Name == "FooBar"
			},
			expected: 3,
		},
	}

	for _, test := range tests {
		got := 0
		impast.ScanDecl(test.pkg, func(decl ast.Decl) bool {
			got++
			return !test.stop(decl)
		})
		if got != test.expected {
			t.Errorf("unexpected decls. expected: %v, but got: %v", test.expected, got)
		}
	}
}

func TestFindTypeByName(t *testing.T) {
	tests := []struct {
		pkg      *ast.Package
		name     string
		expected string
	}{
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type Foo int
`),
				},
			},
			name:     "Foo",
			expected: "int",
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type Foo int
`),
				},
			},
			name:     "Bar",
			expected: "",
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type Foo int
`),
					"bar.go": mustParseFile(`
package main

type Bar float64
`),
				},
			},
			name:     "Bar",
			expected: "float64",
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type (
	Foo int
	Bar float64
	FooBar func(int, int) float64
)
`),
				},
			},
			name:     "FooBar",
			expected: "func(int, int) float64",
		},
	}

	for _, test := range tests {
		typ := impast.FindTypeByName(test.pkg, test.name)
		if got := impast.TypeName(typ); got != test.expected {
			t.Errorf("unexpected type. expected: %q, but got: %q", test.expected, got)
		}
	}
}

func TestTypeName(t *testing.T) {
	tests := []struct {
		expr     ast.Expr
		expected string
	}{
		{
			expr:     nil,
			expected: "",
		},
		{
			expr:     ast.NewIdent("int"),
			expected: "int",
		},
		{
			expr:     &ast.StarExpr{X: ast.NewIdent("Foo")},
			expected: "*Foo",
		},
		{
			expr:     &ast.SelectorExpr{X: ast.NewIdent("test_pkg"), Sel: ast.NewIdent("Bar")},
			expected: "test_pkg.Bar",
		},
		{
			expr:     &ast.ArrayType{Elt: ast.NewIdent("float64")},
			expected: "[]float64",
		},
		{
			expr:     &ast.MapType{Key: ast.NewIdent("string"), Value: ast.NewIdent("int")},
			expected: "map[string]int",
		},
		{
			expr: &ast.ChanType{
				Value: &ast.StructType{Fields: &ast.FieldList{}},
				Dir:   ast.SEND | ast.RECV,
			},
			expected: "chan struct {\n}",
		},
		{
			expr:     &ast.InterfaceType{Methods: &ast.FieldList{}},
			expected: "interface {\n}",
		},
		{
			expr: &ast.FuncType{
				Params: &ast.FieldList{
					List: []*ast.Field{{Type: &ast.ArrayType{Elt: ast.NewIdent("byte")}}},
				},
				Results: &ast.FieldList{
					List: []*ast.Field{
						{Type: ast.NewIdent("int64"), Names: []*ast.Ident{ast.NewIdent("n")}},
						{Type: ast.NewIdent("error"), Names: []*ast.Ident{ast.NewIdent("err")}},
					},
				},
			},
			expected: "func([]byte) (n int64, err error)",
		},
	}

	for _, test := range tests {
		if got := impast.TypeName(test.expr); got != test.expected {
			t.Errorf("unexpected type name. expected: %q, but got: %q", test.expected, got)
		}
	}
}

func TestFindStruct(t *testing.T) {
	tests := []struct {
		pkg      *ast.Package
		name     string
		expected bool
	}{
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type Foo struct {
	bar int
}
`),
				},
			},
			name:     "Foo",
			expected: true,
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type Bar struct {
	bar int
}
`),
				},
			},
			name:     "Foo",
			expected: false,
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type Foo interface {
	Bar()
}
`),
				},
			},
			name:     "Foo",
			expected: false,
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type Foo struct{}
type Bar struct{}
`),
				},
			},
			name:     "Bar",
			expected: true,
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type (
	Foo struct{}
	Bar struct{}
)
`),
				},
			},
			name:     "Bar",
			expected: true,
		},
	}

	for _, test := range tests {
		s := impast.FindStruct(test.pkg, test.name)
		if got := s != nil; got != test.expected {
			t.Errorf("expected result. expected: %v, but got: %v", test.expected, got)
		}
	}
}

func TestFindInterface(t *testing.T) {
	tests := []struct {
		pkg      *ast.Package
		name     string
		expected bool
	}{
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type Foo interface {
	Bar()
}
`),
				},
			},
			name:     "Foo",
			expected: true,
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type Bar interface {
	Foo()
}
`),
				},
			},
			name:     "Foo",
			expected: false,
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type Foo struct {
	bar int
}
`),
				},
			},
			name:     "Foo",
			expected: false,
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type Foo interface{}
type Bar interface{}
`),
				},
			},
			name:     "Bar",
			expected: true,
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type (
	Foo interface{}
	Bar interface{}
)
`),
				},
			},
			name:     "Bar",
			expected: true,
		},
	}

	for _, test := range tests {
		s := impast.FindInterface(test.pkg, test.name)
		if got := s != nil; got != test.expected {
			t.Errorf("expected result. expected: %v, but got: %v", test.expected, got)
		}
	}
}

func TestGetMethods(t *testing.T) {
	tests := []struct {
		pkg      *ast.Package
		name     string
		expected []string
	}{
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type Foo struct {}

func (f Foo) Bar() {}
`),
				},
			},
			name:     "Foo",
			expected: []string{"Bar"},
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type Foo struct {}

func (f *Foo) Bar() {}
`),
				},
			},
			name:     "Foo",
			expected: nil,
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type Foo struct {}

func (f Foo) bar() {}
`),
				},
			},
			name:     "Foo",
			expected: nil,
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type S struct {}

func (s *S) Foo() {}
`),
				},
			},
			name:     "*S",
			expected: []string{"Foo"},
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type S struct {}

func (s *S) Foo() {}
`),
					"foo.go": mustParseFile(`
package main

func (s *S) Bar() {}
`),
				},
			},
			name:     "*S",
			expected: []string{"Bar", "Foo"},
		},
	}

	equals := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	for _, test := range tests {
		methods := impast.GetMethods(test.pkg, test.name)
		var got []string
		for _, method := range methods {
			got = append(got, method.Name.Name)
		}
		sort.Strings(got)
		if !equals(got, test.expected) {
			t.Errorf("unexpected methods. expected: %v, but got: %v", test.expected, got)
		}
	}
}

func TestExportType(t *testing.T) {
	tests := []struct {
		pkg      *ast.Package
		expr     ast.Expr
		expected string
	}{
		{
			pkg:      &ast.Package{Name: "foo"},
			expr:     ast.NewIdent("Foo"),
			expected: "foo.Foo",
		},
		{
			pkg:      &ast.Package{Name: "foo"},
			expr:     ast.NewIdent("bar"),
			expected: "bar",
		},
		{
			pkg:      &ast.Package{Name: "foo"},
			expr:     &ast.StarExpr{X: ast.NewIdent("Foo")},
			expected: "*foo.Foo",
		},
		{
			pkg:      &ast.Package{Name: "foo"},
			expr:     &ast.MapType{Key: ast.NewIdent("Foo"), Value: ast.NewIdent("Bar")},
			expected: "map[foo.Foo]foo.Bar",
		},
		{
			pkg:      &ast.Package{Name: "foo"},
			expr:     &ast.ArrayType{Elt: ast.NewIdent("Bar")},
			expected: "[]foo.Bar",
		},
		{
			pkg:      &ast.Package{Name: "bar"},
			expr:     &ast.ChanType{Value: ast.NewIdent("FooBar"), Dir: ast.RECV | ast.SEND},
			expected: "chan bar.FooBar",
		},
		{
			pkg: &ast.Package{Name: "bar"},
			expr: &ast.FuncType{
				Params: &ast.FieldList{List: []*ast.Field{
					{Type: ast.NewIdent("int")},
					{Type: ast.NewIdent("Foo")},
				}},
				Results: &ast.FieldList{List: []*ast.Field{
					{Type: &ast.StarExpr{X: ast.NewIdent("Bar")}},
					{Type: ast.NewIdent("error")},
				}},
			},
			expected: "func(int, bar.Foo) (*bar.Bar, error)",
		},
		{
			pkg: &ast.Package{Name: "foo"},
			expr: &ast.InterfaceType{
				Methods: &ast.FieldList{List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("Write")},
						Type: &ast.FuncType{
							Params: &ast.FieldList{List: []*ast.Field{
								{Type: &ast.ArrayType{Elt: ast.NewIdent("byte")}},
							}},
							Results: &ast.FieldList{List: []*ast.Field{
								{Type: ast.NewIdent("int64")},
								{Type: ast.NewIdent("error")},
							}},
						},
					},
					{
						Names: []*ast.Ident{ast.NewIdent("FooBar")},
						Type: &ast.FuncType{
							Params: &ast.FieldList{List: []*ast.Field{
								{Type: ast.NewIdent("Foo")},
								{Type: ast.NewIdent("int")},
							}},
							Results: &ast.FieldList{List: []*ast.Field{
								{Type: &ast.StarExpr{X: ast.NewIdent("Bar")}},
							}},
						},
					},
				}},
			},
			expected: `interface {
	Write([]byte) (int64, error)
	FooBar(foo.Foo, int) *foo.Bar
}`,
		},
		{
			pkg: &ast.Package{Name: "foo"},
			expr: &ast.FuncType{
				Params: &ast.FieldList{List: []*ast.Field{
					{Type: &ast.Ellipsis{Elt: ast.NewIdent("BarOption")}, Names: []*ast.Ident{ast.NewIdent("opts")}},
				}},
				Results: &ast.FieldList{},
			},
			expected: "func(opts ...foo.BarOption)",
		},
	}

	for _, test := range tests {
		if got := impast.TypeName(impast.ExportType(test.pkg, test.expr)); got != test.expected {
			t.Errorf("unexpected type. expected: %q, but got: %q", test.expected, got)
		}
	}
}

func TestGetRequires(t *testing.T) {
	tests := []struct {
		pkg      *ast.Package
		name     string
		expected []string
	}{
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type (
	Reader interface {
		Read(p []byte) (n int64, err error)
	}
	Writer interface {
		Write(p []byte) (n int64, err error)
	}
	ReadWriter interface {
		Reader
		Writer
	}
)
`),
				},
			},
			name:     "ReadWriter",
			expected: []string{"Read", "Write"},
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type (
	Reader interface {
		Read(p []byte) (n int64, err error)
	}
	Writer interface {
		Write(p []byte) (n int64, err error)
	}
	ReadWriter interface {
		Reader
		Writer
	}
)
`),
				},
			},
			name:     "Writer",
			expected: []string{"Write"},
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type (
	Foo interface {
		Bar()
	}
	Reader interface {
		Foo
		Read(p []byte) (n int64, err error)
	}
	Writer interface {
		Write(p []byte) (n int64, err error)
	}
	ReadWriter interface {
		Reader
		Writer
	}
)
`),
				},
			},
			name:     "ReadWriter",
			expected: []string{"Bar", "Read", "Write"},
		},
		{
			pkg: &ast.Package{
				Files: map[string]*ast.File{
					"main.go": mustParseFile(`
package main

type (
	Foo interface {
		Bar()
	}
	FooBar interface {
		Bar()
	}
	Reader interface {
		Foo
		Read(p []byte) (n int64, err error)
	}
	Writer interface {
		Write(p []byte) (n int64, err error)
	}
	ReadWriter interface {
		Reader
		Writer
	}
)
`),
				},
			},
			name:     "ReadWriter",
			expected: []string{"Bar", "Read", "Write"},
		},
	}

	equals := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	for _, test := range tests {
		it := impast.FindInterface(test.pkg, test.name)
		if it == nil {
			t.Errorf("interface not found: %q", test.name)
			continue
		}
		fields := impast.GetRequires(it)
		var got []string
		for _, field := range fields {
			got = append(got, field.Names[0].Name)
		}
		sort.Strings(got)
		if !equals(got, test.expected) {
			t.Errorf("unexpected functions. expected: %v, but got: %v", test.expected, got)
		}
	}
}

func TestAutoNaming(t *testing.T) {
	funcType := func(expr string) *ast.FuncType {
		e, err := parser.ParseExpr(expr)
		if err != nil {
			t.Fatal(err)
		}
		return e.(*ast.FuncType)
	}

	tests := []struct {
		in       *ast.FuncType
		expected string
	}{
		{
			in:       funcType("func()"),
			expected: "func()",
		},
		{
			in:       funcType("func(int) int"),
			expected: "func(arg1 int) int",
		},
		{
			in:       funcType("func(int)"),
			expected: "func(arg1 int)",
		},
		{
			in:       funcType("func(int) (int, error)"),
			expected: "func(arg1 int) (int, error)",
		},
		{
			in:       funcType("func(int, int) int"),
			expected: "func(arg1 int, arg2 int) int",
		},
		{
			in:       funcType("func(int) int"),
			expected: "func(arg1 int) int",
		},
		{
			in:       funcType("func(a, b int) int"),
			expected: "func(a, b int) int",
		},
	}

	for _, test := range tests {
		got := impast.TypeName(impast.AutoNaming(test.in))
		if got != test.expected {
			t.Errorf("unexpected function type. expected: %v, but got: %v", test.expected, got)
		}
	}
}

func TestResolveTypeWithCache(t *testing.T) {
	tests := []struct {
		src  *ast.File
		expr ast.Expr
		pkgs map[string]*ast.Package

		expectedPackageName string
		expectedType        string
	}{
		{
			src: mustParseFile(`
package main

import "github.com/example/foobar"
`),
			expr: mustParseExpr("foobar.Type"),
			pkgs: map[string]*ast.Package{
				"github.com/example/foobar": {
					Name: "foobar",
				},
			},

			expectedPackageName: "foobar",
			expectedType:        "Type",
		},
		{
			src: mustParseFile(`
package main

import fb "github.com/example/foobar"
`),
			expr: mustParseExpr("fb.Type"),
			pkgs: map[string]*ast.Package{
				"github.com/example/foobar": {
					Name: "foobar",
				},
			},

			expectedPackageName: "foobar",
			expectedType:        "Type",
		},
		{
			src: mustParseFile(`
package main

import "github.com/example/foobar"
`),
			expr: mustParseExpr("*foobar.Type"),
			pkgs: map[string]*ast.Package{
				"github.com/example/foobar": {
					Name: "foobar",
				},
			},

			expectedPackageName: "foobar",
			expectedType:        "Type",
		},
	}

	for _, test := range tests {
		pkg, name, err := impast.ResolveTypeWithCache(test.src, test.expr, test.pkgs)
		if err != nil {
			t.Error("failed to resolve type")
			continue
		}

		if test.expectedPackageName != "" {
			if pkg == nil || pkg.Name != test.expectedPackageName {
				t.Errorf("unexpected package name. expected: %v, but got: %v", test.expectedPackageName, pkg.Name)
			}
		}
		if name != test.expectedType {
			t.Errorf("unexpected type name. expected: %v, but got: %v", test.expectedType, name)
		}
	}
}

func TestGetMethodsDeepWithCache(t *testing.T) {
	tests := []struct {
		pkg  *ast.Package
		name string
		pkgs map[string]*ast.Package

		expected []string
	}{
		{
			pkg: &ast.Package{
				Name: "foo",
				Files: map[string]*ast.File{
					"foo.go": mustParseFile(`
package foo

type Foo struct {}

func (f Foo) Do(n int) error {
	return nil
}
`),
				},
			},
			name: "Foo",
			pkgs: map[string]*ast.Package{},

			expected: []string{"Do(int)(error)"},
		},
		{
			pkg: &ast.Package{
				Name: "foo",
				Files: map[string]*ast.File{
					"foo.go": mustParseFile(`
package foo

type Foo struct {}

func (f *Foo) Do(n int) error {
	return nil
}
`),
				},
			},
			name: "Foo",
			pkgs: map[string]*ast.Package{},

			expected: []string{"Do(int)(error)"},
		},
		{
			pkg: &ast.Package{
				Name: "foo",
				Files: map[string]*ast.File{
					"foo.go": mustParseFile(`
package foo

type Foo struct {}

func (f *Foo) Do(n int) error {
	return nil
}

func (f Foo) A(msg string) {}
`),
				},
			},
			name: "Foo",
			pkgs: map[string]*ast.Package{},

			expected: []string{"A(string)()", "Do(int)(error)"},
		},
		{
			pkg: &ast.Package{
				Name: "foo",
				Files: map[string]*ast.File{
					"foo.go": mustParseFile(`
package foo

type Foo struct {}

func (f *Foo) Do(n int) error {
	return nil
}
`),
					"foo_a.go": mustParseFile(`
package foo

func (f Foo) A(msg string) {}
`),
				},
			},
			name: "Foo",
			pkgs: map[string]*ast.Package{},

			expected: []string{"A(string)()", "Do(int)(error)"},
		},
		{
			pkg: &ast.Package{
				Name: "foo",
				Files: map[string]*ast.File{
					"foo.go": mustParseFile(`
package foo

import (
	"impast.example/example/bar"
)

type Foo struct {
	bar.Bar
}

func (f *Foo) Do(n int) error {
	return nil
}
`),
					"foo_a.go": mustParseFile(`
package foo

func (f Foo) A(msg string) {}
`),
				},
			},
			name: "Foo",
			pkgs: map[string]*ast.Package{
				"impast.example/example/bar": {
					Name: "bar",
					Files: map[string]*ast.File{
						"bar.go": mustParseFile(`
package bar

type Bar struct {}

func (b *Bar) BarDo(dest interface{}) error {
	return nil
}
`),
					},
				},
			},

			expected: []string{"A(string)()", "BarDo(interface{})(error)", "Do(int)(error)"},
		},
		{
			pkg: &ast.Package{
				Name: "foo",
				Files: map[string]*ast.File{
					"foo.go": mustParseFile(`
package foo

import (
	"impast.example/example/bar"
)

type Foo struct {
	*bar.Bar
}

func (f *Foo) Do(n int) error {
	return nil
}
`),
					"foo_a.go": mustParseFile(`
package foo

func (f Foo) A(msg string) {}
`),
				},
			},
			name: "Foo",
			pkgs: map[string]*ast.Package{
				"impast.example/example/bar": {
					Name: "bar",
					Files: map[string]*ast.File{
						"bar.go": mustParseFile(`
package bar

type Bar struct {}

func (b *Bar) BarDo(dest interface{}) error {
	return nil
}
`),
					},
				},
			},

			expected: []string{"A(string)()", "BarDo(interface{})(error)", "Do(int)(error)"},
		},
		{
			pkg: &ast.Package{
				Name: "foo",
				Files: map[string]*ast.File{
					"foo.go": mustParseFile(`
package foo

import (
	"impast.example/example/bar"
)

type Foo struct {
	Bar bar.Bar
}

func (f *Foo) Do(n int) error {
	return nil
}
`),
					"foo_a.go": mustParseFile(`
package foo

func (f Foo) A(msg string) {}
`),
				},
			},
			name: "Foo",
			pkgs: map[string]*ast.Package{
				"impast.example/example/bar": {
					Name: "bar",
					Files: map[string]*ast.File{
						"bar.go": mustParseFile(`
package bar

type Bar struct {}

func (b *Bar) BarDo(dest interface{}) error {
	return nil
}
`),
					},
				},
			},

			expected: []string{"A(string)()", "Do(int)(error)"},
		},
		{
			pkg: &ast.Package{
				Name: "foo",
				Files: map[string]*ast.File{
					"foo.go": mustParseFile(`
package foo

import (
	"impast.example/example/bar"
)

type Foo struct {
	*bar.Bar
}

func (f *Foo) Do(n int) error {
	return nil
}
`),
					"foo_a.go": mustParseFile(`
package foo

func (f Foo) A(msg string) {}
`),
				},
			},
			name: "Foo",
			pkgs: map[string]*ast.Package{
				"impast.example/example/bar": {
					Name: "bar",
					Files: map[string]*ast.File{
						"bar.go": mustParseFile(`
package bar

type Bar struct {}

func (b *Bar) Do(dest interface{}) error {
	return nil
}
`),
					},
				},
			},

			expected: []string{"A(string)()", "Do(int)(error)"},
		},
	}

	for _, test := range tests {
		methods, err := impast.GetMethodsDeepWithCache(test.pkg, test.name, test.pkgs)
		if err != nil {
			t.Errorf("failed to get methods: %v", err)
			continue
		}
		var got []string
		for _, m := range methods {
			got = append(got, signature(m))
		}
		if !reflect.DeepEqual(got, test.expected) {
			t.Errorf("unexpected signatures. expected: %+v, but got: %+v", test.expected, got)
		}
	}
}

func signature(f *ast.FuncDecl) string {
	return fmt.Sprintf("%v(%v)(%v)", f.Name.Name, types(f.Type.Params), types(f.Type.Results))
}

func types(fl *ast.FieldList) string {
	if fl == nil {
		return ""
	}
	var ts []string
	for _, el := range fl.List {
		b := bytes.NewBuffer(nil)
		printer.Fprint(b, token.NewFileSet(), el.Type)
		t := b.String()
		if len(el.Names) == 0 {
			ts = append(ts, t)
		} else {
			for range el.Names {
				ts = append(ts, t)
			}
		}
	}
	return strings.Join(ts, ",")
}
