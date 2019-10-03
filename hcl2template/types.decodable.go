package hcl2template

import "github.com/hashicorp/hcl/v2/hcldec"

type Decodable interface {
	HCL2Spec() map[string]hcldec.Spec
}
