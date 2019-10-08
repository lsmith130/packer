package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"go/types"
	"io"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/fatih/structtag"
	"github.com/hashicorp/hcl2/hcldec"
	"github.com/zclconf/go-cty/cty"

	"golang.org/x/tools/go/packages"
)

var (
	typeNames  = flag.String("type", "", "comma-separated list of type names; must be set")
	output     = flag.String("output", "", "output file name; default srcdir/<type>_hcl2.go")
	trimprefix = flag.String("trimprefix", "", "trim the `prefix` from the generated constant names")
)

// Usage is a replacement usage function for the flags package.
func Usage() {
	fmt.Fprintf(os.Stderr, "Usage of flatten-mapstructure:\n")
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
		args = []string{os.Getenv("GOPACKAGE")}
	}

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
	sort.Strings(typeNames)

	var structs []StructDef
	usedImports := map[NamePath]*types.Package{}

	for id, obj := range topPkg.TypesInfo.Defs {
		if obj == nil {
			continue
		}
		t := obj.Type()
		nt, isANamedType := t.(*types.Named)
		if !isANamedType {
			continue
		}
		ut := nt.Underlying()
		utStruct, utOk := ut.(*types.Struct)
		if !utOk {
			continue
		}
		pos := sort.SearchStrings(typeNames, id.Name)
		if pos >= len(typeNames) || typeNames[pos] != id.Name {
			continue // not a struct we care about
		}
		flatenedStruct := getMapstructureSquashedStruct(obj.Pkg(), utStruct)
		flatenedStruct = addCtyTagToStruct(flatenedStruct)
		newStructName := "Flat" + id.Name
		structs = append(structs, StructDef{
			OriginalStructName: id.Name,
			StructName:         newStructName,
			Struct:             flatenedStruct,
		})

		for k, v := range getUsedImports(flatenedStruct) {
			if _, found := usedImports[k]; !found {
				usedImports[k] = v
			}
		}
	}

	out := bytes.NewBuffer(nil)

	fmt.Fprintf(out, "package %s\n", topPkg.Name)

	delete(usedImports, NamePath{topPkg.Name, topPkg.PkgPath})
	outputImports(out, usedImports)

	for _, flatenedStruct := range structs {
		fmt.Fprintf(out, "\ntype %s struct {\n", flatenedStruct.StructName)
		outputStructFields(out, flatenedStruct.Struct)
		fmt.Fprint(out, "}\n")

		fmt.Fprintf(out, "\nfunc (*%s) HCL2Spec() map[string]hcldec.Spec {\n", flatenedStruct.OriginalStructName)
		outputStructHCL2SpecBody(out, flatenedStruct.Struct)
		fmt.Fprint(out, "}\n")
	}

	for impt := range usedImports {
		if strings.ContainsAny(impt.Path, "/") {
			out = bytes.NewBuffer(bytes.ReplaceAll(out.Bytes(),
				[]byte(impt.Path+"."),
				[]byte(impt.Name+".")))
		}
	}

	// avoid needing to import current pkg; there's probably a better way.
	out = bytes.NewBuffer(bytes.ReplaceAll(out.Bytes(),
		[]byte(topPkg.PkgPath+"."),
		nil))

	log.Writer().Write(goFmt(out.Bytes()))
}

type StructDef struct {
	OriginalStructName string
	StructName         string
	Struct             *types.Struct
}

func outputStructHCL2SpecBody(w io.Writer, s *types.Struct) {
	fmt.Fprintf(w, "s := map[string]hcldec.Spec{\n")

	for i := 0; i < s.NumFields(); i++ {
		field, tag := s.Field(i), s.Tag(i)
		st, _ := structtag.Parse(tag)
		ctyTag, _ := st.Get("cty")
		fmt.Fprintf(w, "	\"%s\": ", field.Name())
		outputHCL2SpecField(w, ctyTag.Name, field.Type())
		fmt.Fprintln(w, `,`)
	}

	fmt.Fprintln(w, `}`)
	fmt.Fprintln(w, `return s`)
}

func outputHCL2SpecField(w io.Writer, accessor string, fieldType types.Type) {
	switch f := fieldType.(type) {
	case *types.Basic:
		fmt.Fprintf(w, `%#v`, &hcldec.AttrSpec{
			Name:     accessor,
			Type:     basicKindToCtyType(f.Kind()),
			Required: false,
		})
	case *types.Map:
		fmt.Fprintf(w, `%#v`, &hcldec.BlockAttrsSpec{
			TypeName:    accessor,
			ElementType: cty.String, // for now everything can be simplified to a map[string]string
			Required:    false,
		})
	case *types.Slice:
		if basicElem, ok := f.Elem().(*types.Basic); ok {
			fmt.Fprintf(w, `%#v`, &hcldec.AttrSpec{
				Name:     accessor,
				Type:     cty.List(basicKindToCtyType(basicElem.Kind())),
				Required: false,
			})
		} else {
			fmt.Fprintf(w, `nil /* slice (%s) */`, f.String())
		}
	case *types.Named:
		if f.String() == "time.Duration" {
			fmt.Fprintf(w, `%#v`, &hcldec.AttrSpec{
				Name:     accessor,
				Type:     basicKindToCtyType(types.String),
				Required: false,
			})
			return
		}
		underlyingType := f.Underlying()
		switch underlyingType.(type) {
		case *types.Struct:
			fmt.Fprintf(w, `&hcldec.BlockObjectSpec{TypeName: "%[1]s",`+
				` Nested: hcldec.ObjectSpec((*%[1]s)(nil).HCL2Spec())}`, f.String())
		default:
			outputHCL2SpecField(w, accessor, underlyingType)
		}
	default:
		_ = f
		fmt.Fprint(w, `nil /* not basic */`)
	}
}

func basicKindToCtyType(kind types.BasicKind) cty.Type {
	switch kind {
	case types.Bool:
		return cty.Bool
	case types.String:
		return cty.String
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
		return cty.Number
	default:
		panic("blip")
	}
}

func outputStructFields(w io.Writer, s *types.Struct) {
	for i := 0; i < s.NumFields(); i++ {
		field, tag := s.Field(i), s.Tag(i)
		fmt.Fprintf(w, "	%s `%s`\n", strings.Replace(field.String(), "field ", "", 1), tag)
	}
}

type NamePath struct {
	Name, Path string
}

func outputImports(w io.Writer, imports map[NamePath]*types.Package) {
	if len(imports) == 0 {
		return
	}
	// naive implementation
	pkgs := []NamePath{}
	for k := range imports {
		pkgs = append(pkgs, k)
	}
	sort.Slice(pkgs, func(i int, j int) bool {
		return pkgs[i].Path < pkgs[j].Path
	})

	fmt.Fprint(w, "import (\n")
	for _, pkg := range pkgs {
		if pkg.Name == pkg.Path || strings.HasSuffix(pkg.Path, "/"+pkg.Name) {
			fmt.Fprintf(w, "	\"%s\"\n", pkg.Path)
		} else {
			fmt.Fprintf(w, "	%s \"%s\"\n", pkg.Name, pkg.Path)
		}
	}
	fmt.Fprint(w, ")\n")
}

func getUsedImports(s *types.Struct) map[NamePath]*types.Package {
	res := map[NamePath]*types.Package{}
	for i := 0; i < s.NumFields(); i++ {
		fieldType, ok := s.Field(i).Type().(*types.Named)
		if !ok {
			continue
		}
		pkg := fieldType.Obj().Pkg()
		res[NamePath{pkg.Name(), pkg.Path()}] = pkg
	}
	return res
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
func getMapstructureSquashedStruct(topPkg *types.Package, utStruct *types.Struct) *types.Struct {
	res := &types.Struct{}
	for i := 0; i < utStruct.NumFields(); i++ {
		field, tag := utStruct.Field(i), utStruct.Tag(i)
		structtag, _ := structtag.Parse(tag)
		if !field.Exported() {
			continue
		}
		if ms, err := structtag.Get("mapstructure"); err != nil {
			continue //no mapstructure tag
		} else if ms.HasOption("squash") {
			ot := field.Type()
			uot := ot.Underlying()
			utStruct, utOk := uot.(*types.Struct)
			if !utOk {
				continue
			}

			res = squashStructs(res, getMapstructureSquashedStruct(topPkg, utStruct))
			continue
		}
		if field.Pkg() != topPkg {
			field = types.NewField(field.Pos(), topPkg, field.Name(), field.Type(), field.Embedded())
		}
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

func goFmt(b []byte) []byte {
	fb, err := format.Source(b)
	if err != nil {
		log.Printf("formatting err: %v", err)
		return b
	}
	return fb
}
