package main

import (
	"flag"
	"fmt"
	"go/types"
	"io"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/fatih/structtag"

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

	// log.Printf("Loading %v from %v", typeNames, args)

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
	// log.Printf("Package  %#v\n", topPkg)
	sort.Strings(typeNames)

	var structs []StructDef
	var usedImports []*types.Package

	for id, obj := range topPkg.TypesInfo.Defs {
		if obj == nil {
			continue
		}
		t := obj.Type()
		nt, isANamedType := t.(*types.Named)
		if !isANamedType {
			continue
		}
		_ = nt
		ut := t.Underlying()
		utStruct, utOk := ut.(*types.Struct)
		if !utOk {
			continue
		}
		pos := sort.SearchStrings(typeNames, id.Name)
		if pos >= len(typeNames) || typeNames[pos] != id.Name {
			continue
		}
		// log.Printf("%s: %q defines %v\n",
		// 	topPkg.Fset.Position(id.Pos()), id.Name, obj)
		flatenedStruct := getMapstructureSquashedStruct(utStruct)
		flatenedStruct = addCtyTagToStruct(flatenedStruct)
		newStructName := "Flat" + id.Name
		structs = append(structs, StructDef{
			StructName: newStructName,
			Struct:     flatenedStruct,
		})

		usedImports = append(usedImports, getUsedImports(flatenedStruct)...)
	}

	fmt.Fprintf(log.Writer(), "package %s\n", topPkg.Name)

	for _, flatenedStruct := range structs {
		fmt.Fprintf(log.Writer(), "\ntype %s struct {\n", flatenedStruct.StructName)
		outputStruct(log.Writer(), flatenedStruct.Struct)
		fmt.Fprint(log.Writer(), "}\n")
	}

}

type StructDef struct {
	StructName string
	Struct     *types.Struct
}

func outputStruct(w io.Writer, s *types.Struct) {
	for i := 0; i < s.NumFields(); i++ {
		field, tag := s.Field(i), s.Tag(i)
		fmt.Fprintf(w, "	%s `%s`\n", strings.Replace(field.String(), "field ", "", 1), tag)
	}
}

func getUsedImports(s *types.Struct) []*types.Package {
	for i := 0; i < s.NumFields(); i++ {
		field := s.Field(i)
		_ = field
	}
	return nil
}

func addCtyTagToStruct(s *types.Struct) *types.Struct {
	vars, tags := structFields(s)
	for i := range tags {
		field, tag := vars[i], tags[i]
		ctyAccessor := ToSnakeCase(field.Name())
		st, err := structtag.Parse(tag)
		if err == nil {
			if ms, err := st.Get("mapstructure"); err == nil && ms.Name != "" {
				ctyAccessor = ms.Name
			}
		}
		st.Set(&structtag.Tag{Key: "cty", Name: ctyAccessor})
		tags[i] = st.String()
	}
	return types.NewStruct(vars, tags)
}

// getMapstructureSquashedStruct will return the same struct but embedded
// fields with a `mapstructure:",squash"` tag will be un-nested.
func getMapstructureSquashedStruct(utStruct *types.Struct) *types.Struct {
	res := &types.Struct{}
	for i := 0; i < utStruct.NumFields(); i++ {
		field, tag := utStruct.Field(i), utStruct.Tag(i)
		if !field.Exported() {
			continue
		}
		squashed := false
		structtag, _ := structtag.Parse(tag)
		if ms, err := structtag.Get("mapstructure"); err == nil &&
			ms.HasOption("squash") {
			squashed = true
		}
		if squashed {
			squashed = true
			ot := field.Type()
			uot := ot.Underlying()
			utStruct, utOk := uot.(*types.Struct)
			if !utOk {
				continue
			}
			res = squashStructs(res, getMapstructureSquashedStruct(utStruct))
			continue
		}
		// field.
		res = addFieldToStruct(res, field, tag)
	}
	return res
}

func addFieldToStruct(s *types.Struct, field *types.Var, tag string) *types.Struct {
	sf, st := structFields(s)
	return types.NewStruct(append(sf, field), append(st, tag))
}

func squashStructs(a, b *types.Struct) *types.Struct {
	va, ta := structFields(a)
	vb, tb := structFields(b)
	return types.NewStruct(append(va, vb...), append(ta, tb...))
}

func structFields(s *types.Struct) (vars []*types.Var, tags []string) {
	for i := 0; i < s.NumFields(); i++ {
		field, tag := s.Field(i), s.Tag(i)
		vars = append(vars, field)
		tags = append(tags, tag)
	}
	return vars, tags
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func ToSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
