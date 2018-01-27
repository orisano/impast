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
