package hcl2template

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"

	"github.com/hashicorp/packer/helper/communicator"

	amazonebs "github.com/hashicorp/packer/builder/amazon/ebs"
	"github.com/hashicorp/packer/builder/virtualbox/iso"

	"github.com/hashicorp/packer/provisioner/file"
	"github.com/hashicorp/packer/provisioner/shell"

	amazon_import "github.com/hashicorp/packer/post-processor/amazon-import"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func getBasicParser() *Parser {
	return &Parser{
		Parser: hclparse.NewParser(),
		ProvisionersSchemas: map[string]Decodable{
			"shell": &shell.Config{},
			"file":  &file.Config{},
		},
		PostProvisionersSchemas: map[string]Decodable{
			"amazon-import": &amazon_import.Config{},
		},
		CommunicatorSchemas: map[string]Decodable{
			"ssh":   &communicator.SSH{},
			"winrm": &communicator.WinRM{},
		},
		SourceSchemas: map[string]Decodable{
			"amazon-ebs":     &amazonebs.Config{},
			"virtualbox-iso": &iso.Config{},
		},
	}
}

func TestParser_ParseFile(t *testing.T) {
	defaultParser := getBasicParser()

	type fields struct {
		Parser *hclparse.Parser
	}
	type args struct {
		filename string
		cfg      *PackerConfig
	}
	tests := []struct {
		name             string
		parser           *Parser
		args             args
		wantPackerConfig *PackerConfig
		wantDiags        bool
	}{
		{
			"valid " + sourceLabel + " load",
			defaultParser,
			args{"testdata/sources/basic.pkr.hcl", new(PackerConfig)},
			&PackerConfig{
				Sources: map[SourceRef]*Source{
					SourceRef{
						Type: "virtualbox-iso",
						Name: "ubuntu-1204",
					}: {
						Type: "virtualbox-iso",
						Name: "ubuntu-1204",
						Cfg: &iso.FlatConfig{
							HTTPDir:         "xxx",
							ISOChecksum:     "769474248a3897f4865817446f9a4a53",
							ISOChecksumType: "md5",
							RawSingleISOUrl: "http://releases.ubuntu.com/12.04/ubuntu-12.04.5-server-amd64.iso",
							BootCommand:     []string{"..."},
							ShutdownCommand: "echo 'vagrant' | sudo -S shutdown -P now",
							RawBootWait:     "10s",
						},
					},
					SourceRef{
						Type: "amazon-ebs",
						Name: "ubuntu-1604",
					}: {
						Type: "amazon-ebs",
						Name: "ubuntu-1604",
						Cfg:  &amazonebs.FlatConfig{RawRegion: "eu-west-3", InstanceType: "t2.micro"},
					},
					SourceRef{
						Type: "amazon-ebs",
						Name: "that-ubuntu-1.0",
					}: {
						Type: "amazon-ebs",
						Name: "that-ubuntu-1.0",
						Cfg:  &amazonebs.FlatConfig{RawRegion: "eu-west-3", InstanceType: "t2.micro"},
					},
				},
			},
			false,
		},

		{
			"valid " + communicatorLabel + " load",
			defaultParser,
			args{"testdata/communicator/basic.pkr.hcl", new(PackerConfig)},
			&PackerConfig{
				Communicators: map[CommunicatorRef]*Communicator{
					{Type: "ssh", Name: "vagrant"}: {Type: "ssh", Name: "vagrant"},
				},
			},
			false,
		},

		{
			"duplicate " + sourceLabel, defaultParser,
			args{"testdata/sources/basic.pkr.hcl", &PackerConfig{
				Sources: map[SourceRef]*Source{
					SourceRef{
						Type: "amazon-ebs",
						Name: "ubuntu-1604",
					}: {
						Type: "amazon-ebs",
						Name: "ubuntu-1604",
						Cfg:  &amazonebs.FlatConfig{RawRegion: "eu-west-3", InstanceType: "t2.micro"},
					},
				},
			},
			},
			&PackerConfig{
				Sources: map[SourceRef]*Source{
					SourceRef{
						Type: "virtualbox-iso",
						Name: "ubuntu-1204",
					}: {
						Type: "virtualbox-iso",
						Name: "ubuntu-1204",
						Cfg: &iso.FlatConfig{
							HTTPDir:         "xxx",
							ISOChecksum:     "769474248a3897f4865817446f9a4a53",
							ISOChecksumType: "md5",
							RawSingleISOUrl: "http://releases.ubuntu.com/12.04/ubuntu-12.04.5-server-amd64.iso",
							BootCommand:     []string{"..."},
							ShutdownCommand: "echo 'vagrant' | sudo -S shutdown -P now",
							RawBootWait:     "10s",
						},
					},
					SourceRef{
						Type: "amazon-ebs",
						Name: "ubuntu-1604",
					}: {
						Type: "amazon-ebs",
						Name: "ubuntu-1604",
						Cfg:  &amazonebs.FlatConfig{RawRegion: "eu-west-3", InstanceType: "t2.micro"},
					},
					SourceRef{
						Type: "amazon-ebs",
						Name: "that-ubuntu-1.0",
					}: {
						Type: "amazon-ebs",
						Name: "that-ubuntu-1.0",
						Cfg:  &amazonebs.FlatConfig{RawRegion: "eu-west-3", InstanceType: "t2.micro"},
					},
				},
			},
			true,
		},

		{"valid variables load", defaultParser,
			args{"testdata/variables/basic.pkr.hcl", new(PackerConfig)},
			&PackerConfig{
				Variables: PackerV1Variables{
					"image_name": "foo-image-{{user `my_secret`}}",
					"key":        "value",
					"my_secret":  "foo",
				},
			},
			false,
		},

		{"valid " + buildLabel + " load", defaultParser,
			args{"testdata/build/basic.pkr.hcl", new(PackerConfig)},
			&PackerConfig{
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
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.parser
			f, moreDiags := p.ParseHCLFile(tt.args.filename)
			if moreDiags != nil {
				t.Fatalf("diags: %s", moreDiags)
			}
			diags := p.ParseFile(f, tt.args.cfg)
			if tt.wantDiags == (diags == nil) {
				t.Errorf("PackerConfig.Load() unexpected diagnostics. %s", diags)
			}
			if diff := cmp.Diff(tt.wantPackerConfig, tt.args.cfg,
				cmpopts.IgnoreUnexported(cty.Value{}),
				cmpopts.IgnoreTypes(HCL2Ref{}),
				cmpopts.IgnoreTypes([]hcl.Range{}),
				cmpopts.IgnoreTypes(hcl.Range{}),
				cmpopts.IgnoreInterfaces(struct{ hcl.Expression }{}),
				cmpopts.IgnoreInterfaces(struct{ hcl.Body }{}),
			); diff != "" {
				t.Errorf("PackerConfig.Load() wrong packer config. %s", diff)
			}
			if t.Failed() {
				t.Fatal()
			}
		})
	}
}
