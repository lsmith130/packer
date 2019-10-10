package hcl2template

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/hcl/v2"

	amazon_import "github.com/hashicorp/packer/post-processor/amazon-import"
	"github.com/hashicorp/packer/provisioner/file"
	"github.com/hashicorp/packer/provisioner/shell"
	"github.com/zclconf/go-cty/cty"
)

func TestParser_Parse(t *testing.T) {
	defaultParser := getBasicParser()

	type args struct {
		filename string
	}
	tests := []struct {
		name      string
		parser    *Parser
		args      args
		wantCfg   *PackerConfig
		wantDiags bool
	}{
		{"complete",
			defaultParser,
			args{"testdata/complete"},
			&PackerConfig{
				Sources: map[SourceRef]*Source{
					SourceRef{
						Type: "virtualbox-iso",
						Name: "ubuntu-1204",
					}: {
						Type: "virtualbox-iso",
						Name: "ubuntu-1204",
					},
					SourceRef{
						Type: "amazon-ebs",
						Name: "ubuntu-1604",
					}: {
						Type: "amazon-ebs",
						Name: "ubuntu-1604",
					},
					SourceRef{
						Type: "amazon-ebs",
						Name: "that-ubuntu-1.0",
					}: {
						Type: "amazon-ebs",
						Name: "that-ubuntu-1.0",
					},
				},
				Communicators: map[CommunicatorRef]*Communicator{
					{Type: "ssh", Name: "vagrant"}: {Type: "ssh", Name: "vagrant"},
				},
				Variables: PackerV1Variables{
					"image_name": "foo-image-{{user `my_secret`}}",
					"key":        "value",
					"my_secret":  "foo",
				},
				Builds: Builds{
					{
						Froms: BuildFromList{
							{
								Src: SourceRef{"amazon-ebs", "ubuntu-1604"},
							},
							{
								Src: SourceRef{"virtualbox-iso", "ubuntu-1204"},
							},
						},
						ProvisionerGroups: ProvisionerGroups{
							&ProvisionerGroup{
								CommunicatorRef: CommunicatorRef{"ssh", "vagrant"},
								Provisioners: []Provisioner{
									{Cfg: &shell.FlatConfig{
										Inline: []string{"echo '{{user `my_secret`}}' :D"},
									}},
									{Cfg: &shell.FlatConfig{
										Scripts:        []string{"script-1.sh", "script-2.sh"},
										ValidExitCodes: []int{0, 42},
									}},
									{Cfg: &file.FlatConfig{
										Source:      "app.tar.gz",
										Destination: "/tmp/app.tar.gz",
									}},
								},
							},
						},
						PostProvisionerGroups: ProvisionerGroups{
							&ProvisionerGroup{
								Provisioners: []Provisioner{
									{Cfg: &amazon_import.FlatConfig{
										Name: "that-ubuntu-1.0",
									}},
								},
							},
						},
					},
					&Build{
						Froms: BuildFromList{
							{
								Src: SourceRef{"amazon", "that-ubuntu-1"},
							},
						},
						ProvisionerGroups: ProvisionerGroups{
							&ProvisionerGroup{
								Provisioners: []Provisioner{
									{Cfg: &shell.FlatConfig{
										Inline: []string{"echo HOLY GUACAMOLE !"},
									}},
								},
							},
						},
					},
				},
			}, false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCfg, gotDiags := tt.parser.Parse(tt.args.filename)
			if tt.wantDiags == (gotDiags == nil) {
				t.Errorf("Parser.Parse() unexpected diagnostics. %s", gotDiags)
			}
			if diff := cmp.Diff(tt.wantCfg, gotCfg,
				cmpopts.IgnoreUnexported(cty.Value{}),
				cmpopts.IgnoreTypes(HCL2Ref{}),
				cmpopts.IgnoreTypes([]hcl.Range{}),
				cmpopts.IgnoreTypes(hcl.Range{}),
				cmpopts.IgnoreInterfaces(struct{ hcl.Expression }{}),
				cmpopts.IgnoreInterfaces(struct{ hcl.Body }{}),
			); diff != "" {
				t.Errorf("Parser.Parse() wrong packer config. %s", diff)
			}

		})
	}
}
