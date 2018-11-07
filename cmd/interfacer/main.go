package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"log"
	"os"
	"strconv"
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

		nameCache := map[string]string{}
	}
}
