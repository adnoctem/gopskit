package tools

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	fs "github.com/adnoctem/gopskit/pkg/fsi"
	"github.com/adnoctem/gopskit/pkg/helpers"
	"github.com/adnoctem/gopskit/pkg/proc"
	"gopkg.in/yaml.v3"
)

const (
	SmallstepDefaultName           = "Ad Noctem Collective Private Certificate Authority"
	SmallstepDefaultHostname       = "ca.mvprowess.com"
	SmallstepDefaultAddress        = "0.0.0.0:443"
	SmallstepDefaultProvisioner    = "info@mvprowess.com"
	SmallstepDefaultDeploymentType = "standalone"
)

type StepValuesOpt func(cfg *StepValuesConfig)

type StepValuesConfig struct {
	Name           string
	Hostname       string
	Address        string
	Provisioner    string
	DeploymentType string
	Passphrase     string
}

func WithName(name string) func(cfg *StepValuesConfig) {
	return func(cfg *StepValuesConfig) {
		cfg.Name = name
	}
}

func WithHostname(hostname string) func(cfg *StepValuesConfig) {
	return func(cfg *StepValuesConfig) {
		cfg.Hostname = hostname
	}
}

func WithAddress(address string) func(cfg *StepValuesConfig) {
	return func(cfg *StepValuesConfig) {
		cfg.Address = address
	}
}

func WithProvisioner(provisioner string) func(cfg *StepValuesConfig) {
	return func(cfg *StepValuesConfig) {
		cfg.Provisioner = provisioner
	}
}

func WithDeploymentType(deploymentType string) func(cfg *StepValuesConfig) {
	return func(cfg *StepValuesConfig) {
		cfg.DeploymentType = deploymentType
	}
}

func WithPassphrase(passphrase string) func(cfg *StepValuesConfig) {
	return func(cfg *StepValuesConfig) {
		cfg.Passphrase = passphrase
	}
}

func GenerateStepValues(opts ...StepValuesOpt) (*StepHelmValues, string, error) {
	// sanity
	_, err := proc.LookPath("step")
	if err != nil {
		return nil, "", fmt.Errorf("step CLI is not installed or available in PATH. cannot continue")
	}

	// init
	cfg := &StepValuesConfig{
		Name:           SmallstepDefaultName,
		Hostname:       SmallstepDefaultHostname,
		Address:        SmallstepDefaultAddress,
		Provisioner:    SmallstepDefaultProvisioner,
		DeploymentType: SmallstepDefaultDeploymentType,
		Passphrase:     "",
	}

	// configure
	for _, o := range opts {
		o(cfg)
	}

	if cfg.Passphrase == "" {
		cfg.Passphrase = helpers.GeneratePassphrase(helpers.WithLength(64))
	}
	pwB64 := base64.StdEncoding.EncodeToString([]byte(cfg.Passphrase))

	tmp, err := fs.TempDir("step")
	if err != nil {
		return nil, "", err
	}

	pwPath := filepath.Join(tmp, "step-ca-password.txt")
	if err := fs.Write(pwPath, []byte(pwB64)); err != nil {
		return nil, "", err
	}

	valPath := filepath.Join(tmp, "step-values.yaml")
	args := []string{
		"step",
		"ca",
		"init",
		"--helm",
		"--password-file",
		pwPath,
		"--deployment-type",
		cfg.DeploymentType,
		"--name",
		cfg.Name,
		"--dns",
		cfg.Hostname,
		"--address",
		cfg.Address,
		"--provisioner",
		cfg.Provisioner,
	}
	e, err := proc.NewExecutor()
	if err != nil {
		return nil, "", err
	}

	_, err = e.Execute(args, proc.WithOutputs(valPath))
	if err != nil {
		return nil, "", err
	}

	var data = &StepHelmValues{}
	content, err := os.ReadFile(valPath)
	if err != nil {
		return nil, "", err
	}

	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, "", err
	}

	return data, cfg.Passphrase, nil
}

// StepHelmValues is the Go struct representation of the Smallstep (CLI)'s YAML
// template for Helm. See `smallstep/cli` on GitHub for more information.
type StepHelmValues struct {
	Inject struct {
		Enabled bool `yaml:"enabled"`
		Config  struct {
			Files struct {
				CaJSON struct {
					Root          string        `yaml:"root"`
					FederateRoots []interface{} `yaml:"federateRoots"`
					Crt           string        `yaml:"crt"`
					Key           string        `yaml:"key"`
					Address       string        `yaml:"address"`
					DNSNames      []string      `yaml:"dnsNames"`
					Logger        struct {
						Format string `yaml:"format"`
					} `yaml:"logger"`
					Db struct {
						Type       string `yaml:"type"`
						DataSource string `yaml:"dataSource"`
					} `yaml:"db"`
					Authority struct {
						EnableAdmin  bool `yaml:"enableAdmin"`
						Provisioners []struct {
							Type string `yaml:"type"`
							Name string `yaml:"name"`
							Key  struct {
								Use string `yaml:"use"`
								Kty string `yaml:"kty"`
								Kid string `yaml:"kid"`
								Crv string `yaml:"crv"`
								Alg string `yaml:"alg"`
								X   string `yaml:"x"`
								Y   string `yaml:"y"`
							} `yaml:"key"`
							EncryptedKey string `yaml:"encryptedKey"`
							Options      struct {
								X509 struct {
								} `yaml:"x509"`
								SSH struct {
								} `yaml:"ssh"`
							} `yaml:"options"`
						} `yaml:"provisioners"`
					} `yaml:"authority"`
					TLS struct {
						CipherSuites  []string `yaml:"cipherSuites"`
						MinVersion    float64  `yaml:"minVersion"`
						MaxVersion    float64  `yaml:"maxVersion"`
						Renegotiation bool     `yaml:"renegotiation"`
					} `yaml:"tls"`
				} `yaml:"ca.json"`
				DefaultsJSON struct {
					CaURL       string `yaml:"ca-url"`
					CaConfig    string `yaml:"ca-config"`
					Fingerprint string `yaml:"fingerprint"`
					Root        string `yaml:"root"`
				} `yaml:"defaults.json"`
			} `yaml:"files"`
		} `yaml:"config"`
		Certificates struct {
			IntermediateCa string `yaml:"intermediate_ca"`
			RootCa         string `yaml:"root_ca"`
		} `yaml:"certificates"`
		Secrets struct {
			CaPassword          interface{} `yaml:"ca_password"`
			ProvisionerPassword interface{} `yaml:"provisioner_password"`
			X509                struct {
				IntermediateCaKey string `yaml:"intermediate_ca_key"`
				RootCaKey         string `yaml:"root_ca_key"`
			} `yaml:"x509"`
		} `yaml:"secrets"`
	} `yaml:"inject"`
}

// AddSecretStepValues adds the newly generated StepValues to the secret encrypted
// environment values managed via Helmfile
func AddSecretStepValues(values *StepHelmValues, password, path string) (map[string]interface{}, error) {
	mp, err := AddSecretValue(path, map[string]interface{}{
		"step-ca": map[string]interface{}{
			"ca_password":         password,
			"root_ca":             values.Inject.Certificates.RootCa,
			"root_ca_key":         values.Inject.Secrets.X509.RootCaKey,
			"intermediate_ca":     values.Inject.Certificates.IntermediateCa,
			"intermediate_ca_key": values.Inject.Secrets.X509.IntermediateCaKey,
			"provisioners":        []interface{}{values.Inject.Config.Files.CaJSON.Authority.Provisioners[0]},
			"fingerprint":         values.Inject.Config.Files.DefaultsJSON.Fingerprint,
		},
	}, true)

	if err != nil {
		return nil, err
	}

	return mp, nil
}
