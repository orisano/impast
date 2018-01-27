package pkgast_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/orisano/pkgast"
)

func mustParseFile(src string) *ast.File {
	f, err := parser.ParseFile(token.NewFileSet(), "", src, 0)
	if err != nil {
		panic(err)
	}
	return f
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
		pkgast.ScanDecl(test.pkg, func(decl ast.Decl) bool {
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
		typ := pkgast.FindTypeByName(test.pkg, test.name)
		if got := pkgast.TypeName(typ); got != test.expected {
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
		if got := pkgast.TypeName(test.expr); got != test.expected {
			t.Errorf("unexpected type name. expected: %q, but got: %q", test.expected, got)
		}
	}
}
