package hcl2template

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	"github.com/zclconf/go-cty/cty/json"
)

type ProvisionerGroup struct {
	CommunicatorRef CommunicatorRef

	Provisioners []cty.Value
	HCL2Ref      HCL2Ref
}

// ProvisionerGroups is a slice of provision blocks; which contains
// provisioners
type ProvisionerGroups []*ProvisionerGroup

func (p *Parser) decodeProvisionerGroup(block *hcl.Block, provisionerSpecs map[string]Decodable) (*ProvisionerGroup, hcl.Diagnostics) {
	var b struct {
		Communicator string   `hcl:"communicator,optional"`
		Remain       hcl.Body `hcl:",remain"`
	}

	diags := gohcl.DecodeBody(block.Body, nil, &b)

	pg := &ProvisionerGroup{}
	pg.CommunicatorRef = communicatorRefFromString(b.Communicator)
	pg.HCL2Ref.DeclRange = block.DefRange

	buildSchema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{},
	}
	for k := range provisionerSpecs {
		buildSchema.Blocks = append(buildSchema.Blocks, hcl.BlockHeaderSchema{
			Type: k,
		})
	}

	content, moreDiags := b.Remain.Content(buildSchema)
	diags = append(diags, moreDiags...)
	for _, block := range content.Blocks {
		provisioner, found := provisionerSpecs[block.Type]
		if !found {
			continue
		}
		spec := provisioner.HCL2Spec()
		cv, moreDiags := hcldec.Decode(block.Body, hcldec.ObjectSpec(spec), nil)
		bytes, err := json.SimpleJSONValue{cv}.MarshalJSON()
		if err != nil {
			panic("TODO(azr): error properly")
		}
		str := string(bytes)
		_ = str
		diags = append(diags, moreDiags...)
		err = gocty.FromCtyValue(cv, provisioner)
		if err != nil {
			diags = append(diags, &hcl.Diagnostic{
				Summary: err.Error(),
				Subject: &block.DefRange,
			})
		}
		pg.Provisioners = append(pg.Provisioners, cv)
	}

	return pg, diags
}

func (pgs ProvisionerGroups) FirstCommunicatorRef() CommunicatorRef {
	if len(pgs) == 0 {
		return NoCommunicator
	}
	return pgs[0].CommunicatorRef
}
