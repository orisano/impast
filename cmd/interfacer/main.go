package main

import (
	"flag"
	"go/ast"
	"go/printer"
	"go/token"
	"log"
	"os"
	"strings"

	"github.com/orisano/impast"
)

func main() {
	interfaceName := flag.String("out", "", "generate interface name (required)")
	flag.Parse()

	log.SetFlags(0)
	log.SetPrefix("interfacer: ")

	if *interfaceName == "" {
		log.Print("-out is must be required")
		flag.Usage()
		os.Exit(2)
	}

	var m []*ast.FuncDecl
	pkgs := map[string]*ast.Package{}
	for _, t := range flag.Args() {
		index := strings.LastIndexByte(t, '.')
		if index == -1 {
			log.Fatalf("invalid type: %v", t)
		}
		pkgPath := t[:index]
		typeName := t[index+1:]

		pkg, err := impast.ImportPackage(pkgPath)
		if err != nil {
			log.Fatalf("failed to import package (%v): %v", pkgPath, err)
		}

		methods, err := impast.GetMethodsDeepWithCache(pkg, typeName, pkgs)
		if err != nil {
			log.Fatalf("failed to get methods %v.%v: %v", pkg.Name, typeName, err)
		}
		m = intersectionMethods(m, methods)
	}

	it := &ast.InterfaceType{
		Methods: &ast.FieldList{},
	}
	for _, method := range m {
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
				Type: it,
			},
		},
	}
	printer.Fprint(os.Stdout, token.NewFileSet(), decl)
	os.Stdout.WriteString("\n")
}

func intersectionMethods(a, b []*ast.FuncDecl) []*ast.FuncDecl {
	if a == nil {
		return b
	}
	c := a[:0]
	for _, x := range a {
		for len(b) > 0 && b[0].Name.Name < x.Name.Name {
			b = b[1:]
		}
		if len(b) > 0 && b[0].Name.Name == x.Name.Name {
			c = append(c, x)
		}
	}
	return c
}
