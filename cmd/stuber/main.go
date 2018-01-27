package main

import (
	"flag"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"

	"github.com/orisano/pkgast"
)

func main() {
	pkgPath := flag.String("pkg", "", "package path")
	interfaceName := flag.String("implement", "", "implement interface name")
	typeName := flag.String("type", "", "type name")
	receiverName := flag.String("name", "", "receiver name")
	flag.Parse()

	pkg, err := pkgast.ImportPackage(*pkgPath)
	if err != nil {
		log.Fatal(err)
	}

	it := pkgast.FindInterface(pkg, *interfaceName)
	if it == nil {
		log.Fatalf("interface not found %q", *interfaceName)
	}

	body, err := parser.ParseExpr(`panic("implement me")`)
	if err != nil {
		panic(err)
	}

	for _, method := range pkgast.GetRequires(it) {
		decl := &ast.FuncDecl{
			Name: method.Names[0],
			Recv: &ast.FieldList{List: []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent(*receiverName)},
					Type:  ast.NewIdent(*typeName),
				},
			}},
			Type: method.Type.(*ast.FuncType),
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.ExprStmt{X: body},
			}},
		}
		printer.Fprint(os.Stdout, token.NewFileSet(), decl)
		os.Stdout.WriteString("\n\n")
	}
}
