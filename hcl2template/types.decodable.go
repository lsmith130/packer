package hcl2template

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
)

type Decodable interface {
	HCL2Spec() (specMap map[string]hcldec.Spec)
	FlatMapstructure() (flatennedStruct interface{})
}

func decodeDecodable(body hcl.Body, ctx *hcl.EvalContext, dec Decodable) (interface{}, hcl.Diagnostics) {
	flatCfg := dec.FlatMapstructure()

	// val, moreDiags := hcldec.Decode(block.Body, hcldec.ObjectSpec(spec), nil)
	// diags = append(diags, moreDiags...)

	diags := gohcl.DecodeBody(body, ctx, flatCfg)
	return flatCfg, diags

	// err := gocty.FromCtyValue(val, flatProvisinerCfg)
	// if err != nil {
	// 	diags = append(diags, &hcl.Diagnostic{
	// 		Summary: "gocty.FromCtyValue: " + err.Error(),
	// 		Subject: &block.DefRange,
	// 	})
	// }
}
