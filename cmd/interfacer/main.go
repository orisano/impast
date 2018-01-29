package main

import (
	"flag"
	"go/ast"
	"go/printer"
	"go/token"
	"log"
	"os"

	"github.com/orisano/impast"
)

func main() {
	pkgPath := flag.String("pkg", "", "package path")
	typeName := flag.String("type", "", "type name")
	interfaceName := flag.String("out", "", "generate interface name")
	flag.Parse()

	pkg, err := impast.ImportPackage(*pkgPath)
	if err != nil {
		log.Fatal(err)
	}

	it := &ast.InterfaceType{
		Methods: &ast.FieldList{},
	}
	methods := impast.GetMethods(pkg, *typeName)
	for _, method := range methods {
		it.Methods.List = append(it.Methods.List, &ast.Field{
			Type:  method.Type,
			Names: []*ast.Ident{method.Name},
		})
	}
	decl := &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{
				Name: ast.NewIdent(*interfaceName),
				Type: impast.ExportType(pkg, it),
			},
		},
	}
	printer.Fprint(os.Stdout, token.NewFileSet(), decl)
	os.Stdout.WriteString("\n")
}
