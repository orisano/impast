package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"log"
	"os"

	"github.com/orisano/impast"
)

func main() {
	pkgPath := flag.String("pkg", "", "package path")
	interfaceName := flag.String("type", "", "interface type")
	flag.Parse()

	pkg, err := impast.ImportPackage(*pkgPath)
	if err != nil {
		log.Fatal(err)
	}

	it := impast.FindInterface(pkg, *interfaceName)
	if it == nil {
		log.Fatalf("interface not found %q", *interfaceName)
	}

	mockName := ast.NewIdent(*interfaceName + "Mock")
	st := &ast.StructType{Fields: &ast.FieldList{}}
	methods := impast.GetRequires(it)
	for i := range methods {
		methods[i].Type = impast.ExportType(pkg, methods[i].Type)
	}
	for _, method := range methods {
		st.Fields.List = append(st.Fields.List, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(method.Names[0].Name + "Mock")},
			Type:  method.Type,
		})
	}
	genDecl := &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{&ast.TypeSpec{
			Type: st,
			Name: mockName,
		}},
	}
	printer.Fprint(os.Stdout, token.NewFileSet(), genDecl)
	os.Stdout.WriteString("\n\n")

	recvName := ast.NewIdent("mo")

	for _, method := range methods {
		ft := autoNaming(method.Type.(*ast.FuncType))
		expr := &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recvName, Sel: ast.NewIdent(method.Names[0].Name + "Mock")},
			Args: flattenName(ft.Params),
		}
		if len(ft.Params.List) >= 1 {
			if _, variadic := ft.Params.List[len(ft.Params.List)-1].Type.(*ast.Ellipsis); variadic {
				expr.Ellipsis = token.Pos(1)
			}
		}
		var stmt ast.Stmt
		if ft.Results == nil {
			stmt = &ast.ExprStmt{X: expr}
		} else {
			stmt = &ast.ReturnStmt{Results: []ast.Expr{expr}}
		}

		funcDecl := &ast.FuncDecl{
			Name: method.Names[0],
			Recv: &ast.FieldList{List: []*ast.Field{
				{
					Names: []*ast.Ident{recvName},
					Type:  &ast.StarExpr{X: mockName},
				},
			}},
			Type: ft,
			Body: &ast.BlockStmt{List: []ast.Stmt{stmt}},
		}
		printer.Fprint(os.Stdout, token.NewFileSet(), funcDecl)
		os.Stdout.WriteString("\n\n")
	}
}

func autoNaming(ft *ast.FuncType) *ast.FuncType {
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

func flattenName(fields *ast.FieldList) []ast.Expr {
	var names []ast.Expr
	for _, field := range fields.List {
		for _, name := range field.Names {
			names = append(names, name)
		}
	}
	return names
}
