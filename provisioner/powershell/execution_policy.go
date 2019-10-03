//go:generate enumer -transform snake -trimprefix ExecutionPolicy -type ExecutionPolicy

package powershell

import (
	"reflect"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/zclconf/go-cty/cty"
)

// ExecutionPolicy setting to run the command(s).
// For the powershell provider the default has historically been to bypass.
type ExecutionPolicy int

const (
	ExecutionPolicyBypass ExecutionPolicy = iota
	ExecutionPolicyAllsigned
	ExecutionPolicyDefault
	ExecutionPolicyRemotesigned
	ExecutionPolicyRestricted
	ExecutionPolicyUndefined
	ExecutionPolicyUnrestricted
	ExecutionPolicyNone // not set
)

func StringToExecutionPolicyHook(f reflect.Kind, t reflect.Kind, data interface{}) (interface{}, error) {
	if f != reflect.String || t != reflect.Int {
		return data, nil
	}

	raw := data.(string)
	return ExecutionPolicyString(raw)
}

func (*ExecutionPolicy) HCL2Spec() *hcldec.AttrSpec {
	return &hcldec.AttrSpec{Name: "execution_policy", Type: cty.String, Required: false}
}
