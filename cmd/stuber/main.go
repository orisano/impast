package main

import (
	"flag"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"

	"github.com/orisano/impast"
)

func main() {
	pkgPath := flag.String("pkg", "", "package path")
	interfaceName := flag.String("implement", "", "implement interface name")
	typeName := flag.String("type", "", "type name")
	receiverName := flag.String("name", "", "receiver name")
	export := flag.Bool("export", false, "export")
	flag.Parse()

	pkg, err := impast.ImportPackage(*pkgPath)
	if err != nil {
		log.Fatal(err)
	}

	it := impast.FindInterface(pkg, *interfaceName)
	if it == nil {
		log.Fatalf("interface not found %q", *interfaceName)
	}

	body, err := parser.ParseExpr(`panic("implement me")`)
	if err != nil {
		panic(err)
	}

	for _, method := range impast.GetRequires(it) {
		t := method.Type
		if *export {
			t = impast.ExportType(pkg, t)
		}
		decl := &ast.FuncDecl{
			Name: method.Names[0],
			Recv: &ast.FieldList{List: []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent(*receiverName)},
					Type:  ast.NewIdent(*typeName),
				},
			}},
			Type: impast.AutoNaming(t.(*ast.FuncType)),
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.ExprStmt{X: body},
			}},
		}
		printer.Fprint(os.Stdout, token.NewFileSet(), decl)
		os.Stdout.WriteString("\n\n")
	}
}
