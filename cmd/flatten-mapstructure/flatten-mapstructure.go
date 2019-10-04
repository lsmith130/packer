package main

import (
	"flag"
	"fmt"
	"go/types"
	"log"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

var (
	typeNames  = flag.String("type", "", "comma-separated list of type names; must be set")
	output     = flag.String("output", "", "output file name; default srcdir/<type>_hcl2.go")
	trimprefix = flag.String("trimprefix", "", "trim the `prefix` from the generated constant names")
)

// Usage is a replacement usage function for the flags package.
func Usage() {
	fmt.Fprintf(os.Stderr, "Usage of stringer:\n")
	fmt.Fprintf(os.Stderr, "\tflatten-mapstructure [flags] -type T[,T...] pkg\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	flag.PrintDefaults()
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("flatten-mapstructure: ")
	flag.Usage = Usage
	flag.Parse()
	if len(*typeNames) == 0 {
		flag.Usage()
		os.Exit(2)
	}
	typeNames := strings.Split(*typeNames, ",")

	// We accept either one directory or a list of files. Which do we have?
	args := flag.Args()
	if len(args) == 0 {
		// Default: process whole package in current directory.
		args = []string{os.Getenv("GOFILE")}
	}

	log.Printf("Loading %v from %v", typeNames, args)

	cfg := &packages.Config{
		Mode: packages.LoadSyntax,
	}
	pkgs, err := packages.Load(cfg, args...)
	if err != nil {
		log.Fatal(err)
	}
	if len(pkgs) != 1 {
		log.Fatalf("error: %d packages found", len(pkgs))
	}
	topPkg := pkgs[0]
	fmt.Printf("Package  %#v\n", topPkg)
	sort.Strings(typeNames)
	for id, obj := range topPkg.TypesInfo.Defs {
		if obj == nil {
			continue
		}
		t := obj.Type()
		ut := t.Underlying()
		utStruct, utOk := ut.(*types.Struct)
		if !utOk {
			continue
		}
		pos := sort.SearchStrings(typeNames, id.Name)
		if pos >= len(typeNames) || typeNames[pos] != id.Name {
			continue
		}

		log.Printf("%s: %q defines %v\n",
			topPkg.Fset.Position(id.Pos()), id.Name, obj)

		for i := 0; i < utStruct.NumFields(); i++ {
			field, tag := utStruct.Field(i), utStruct.Tag(i)
			_, _ = field, tag
			if !field.Exported() {
				continue
			}
			ot := field.Type()
			uot := ot.Underlying()
			embedded := field.Embedded()
			_ = embedded
			_ = uot
			_ = ot
			// field.
			println("")
		}
	}
}
