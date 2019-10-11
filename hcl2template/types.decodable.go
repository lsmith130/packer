package hcl2template

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

type Decodable interface {
	HCL2Spec() (specMap map[string]hcldec.Spec)
	FlatMapstructure() (flatennedStruct interface{})
}

func decodeDecodable(block *hcl.Block, ctx *hcl.EvalContext, dec Decodable) (interface{}, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	decodeGoBody := false
	if decodeGoBody {
		flatCfg := dec.FlatMapstructure()
		return flatCfg, gohcl.DecodeBody(block.Body, ctx, flatCfg)
	}
	spec := dec.HCL2Spec()
	val, moreDiags := hcldec.Decode(block.Body, hcldec.ObjectSpec(spec), ctx)
	diags = append(diags, moreDiags...)

	flatProvisinerCfg := dec.FlatMapstructure()

	err := gocty.FromCtyValue(val, flatProvisinerCfg)
	if err != nil {
		switch err := err.(type) {
		case cty.PathError:
			diags = append(diags, &hcl.Diagnostic{
				Summary: "gocty.FromCtyValue: " + err.Error(),
				Subject: &block.DefRange,
				Detail:  fmt.Sprintf("%v", err.Path),
			})
		default:
			diags = append(diags, &hcl.Diagnostic{
				Summary: "gocty.FromCtyValue: " + err.Error(),
				Subject: &block.DefRange,
				Detail:  fmt.Sprintf("%v", err),
			})
		}
	}
	return flatProvisinerCfg, diags
}
