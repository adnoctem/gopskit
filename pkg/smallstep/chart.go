package smallstep

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ChartName is the name of Smallstep's Helm chart, which its resource names derive from.
const ChartName = "step-certificates"

// Fullname is the prefix the step-certificates chart gives its resources for a release, following
// its "step-certificates.fullname" helper (without a fullnameOverride or nameOverride).
func Fullname(release string) string {
	name := release
	if !strings.Contains(release, ChartName) {
		name = release + "-" + ChartName
	}

	if len(name) > 63 {
		name = name[:63]
	}

	return strings.TrimSuffix(name, "-")
}

// HelmValues returns chart values that inject this CA - the chart's "inject" mode, what
// `step ca init --helm` produces - with every password already included. The root key is only
// included with includeRootKey: step-ca never needs it, and it is safest kept out of the cluster.
func (p *PKI) HelmValues(includeRootKey bool) map[string]any {
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

	rootKey := ""
	if includeRootKey {
		rootKey = string(p.RootKey)
	}

	certificates := map[string]any{
		"root_ca":         string(p.RootCert),
		"intermediate_ca": string(p.IntermediateCert),
	}
	secrets := map[string]any{
		"ca_password":          b64(p.Options.Passwords.CA),
		"provisioner_password": b64(p.Options.Passwords.Provisioner),
		"x509": map[string]any{
			"enabled":             true,
			"intermediate_ca_key": string(p.IntermediateKey),
			"root_ca_key":         rootKey,
		},
		"ssh": map[string]any{"enabled": false},
	}

	if p.Options.SSH {
		certificates["ssh_host_ca"] = strings.TrimSpace(string(p.SSHHostPublicKey))
		certificates["ssh_user_ca"] = strings.TrimSpace(string(p.SSHUserPublicKey))
		secrets["ssh"] = map[string]any{
			"enabled":          true,
			"host_ca_key":      string(p.SSHHostKey),
			"host_ca_password": b64(p.Options.Passwords.SSHHost),
			"user_ca_key":      string(p.SSHUserKey),
			"user_ca_password": b64(p.Options.Passwords.SSHUser),
		}
	}

	return map[string]any{
		"inject": map[string]any{
			"enabled": true,
			"config": map[string]any{
				"files": map[string]any{
					"ca.json":       p.CAConfig,
					"defaults.json": p.Defaults,
				},
			},
			"certificates": certificates,
			"secrets":      secrets,
		},
	}
}

// ExistingSecretsValues returns the chart values that make a release use the Secrets from
// Secrets instead of generating or injecting its own.
func (p *PKI) ExistingSecretsValues() map[string]any {
	return map[string]any{
		"existingSecrets": map[string]any{
			"enabled":        true,
			"ca":             true,
			"certsAsSecret":  true,
			"configAsSecret": true,
			"sshHostCa":      p.Options.SSH,
			"sshUserCa":      p.Options.SSH,
		},
		"bootstrap": map[string]any{
			"enabled":    false,
			"secrets":    false,
			"configmaps": false,
		},
		"inject": map[string]any{"enabled": false},
	}
}

// Secrets returns the Secrets the chart's "existingSecrets" mode reads, named for release in
// namespace - for GitOps setups where the CA must exist before the chart is installed. The root
// key is only included with includeRootKey.
func (p *PKI) Secrets(release, namespace string, includeRootKey bool) ([]*corev1.Secret, error) {
	prefix := Fullname(release)
	secret := func(suffix string, typ corev1.SecretType, data map[string]string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      prefix + "-" + suffix,
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/name":       ChartName,
					"app.kubernetes.io/instance":   release,
					"app.kubernetes.io/managed-by": "steppa",
				},
			},
			Type:       typ,
			StringData: data,
		}
	}

	caJSON, err := json.MarshalIndent(p.CAConfig, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("could not encode ca.json: %w", err)
	}
	defaultsJSON, err := json.MarshalIndent(p.Defaults, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("could not encode defaults.json: %w", err)
	}

	keys := map[string]string{"intermediate_ca_key": string(p.IntermediateKey)}
	certs := map[string]string{
		"root_ca.crt":         string(p.RootCert),
		"intermediate_ca.crt": string(p.IntermediateCert),
	}
	if includeRootKey {
		keys["root_ca_key"] = string(p.RootKey)
	}
	if p.Options.SSH {
		keys["ssh_host_ca_key"] = string(p.SSHHostKey)
		keys["ssh_user_ca_key"] = string(p.SSHUserKey)
		certs["ssh_host_ca_key.pub"] = string(p.SSHHostPublicKey)
		certs["ssh_user_ca_key.pub"] = string(p.SSHUserPublicKey)
	}

	secrets := []*corev1.Secret{
		secret("secrets", "smallstep.com/private-keys", keys),
		secret("certs", corev1.SecretTypeOpaque, certs),
		secret("config", corev1.SecretTypeOpaque, map[string]string{
			"ca.json":       string(caJSON),
			"defaults.json": string(defaultsJSON),
		}),
		secret("ca-password", "smallstep.com/ca-password", map[string]string{"password": p.Options.Passwords.CA}),
		secret("provisioner-password", "smallstep.com/provisioner-password",
			map[string]string{"password": p.Options.Passwords.Provisioner}),
	}

	if p.Options.SSH {
		secrets = append(secrets,
			secret("ssh-host-ca-password", "smallstep.com/ssh-host-ca-password",
				map[string]string{"password": p.Options.Passwords.SSHHost}),
			secret("ssh-user-ca-password", "smallstep.com/ssh-user-ca-password",
				map[string]string{"password": p.Options.Passwords.SSHUser}),
		)
	}

	return secrets, nil
}

// Bundle is everything the CA consists of in one flat map - for a secrets store such as Vault,
// where the CA can be recovered from, including the root key and every password.
func (p *PKI) Bundle() map[string]string {
	caJSON, _ := json.Marshal(p.CAConfig)
	defaultsJSON, _ := json.Marshal(p.Defaults)

	b := map[string]string{
		"root_ca.crt":          string(p.RootCert),
		"root_ca_key":          string(p.RootKey),
		"intermediate_ca.crt":  string(p.IntermediateCert),
		"intermediate_ca_key":  string(p.IntermediateKey),
		"fingerprint":          p.Fingerprint,
		"ca.json":              string(caJSON),
		"defaults.json":        string(defaultsJSON),
		"root_password":        p.Options.Passwords.Root,
		"ca_password":          p.Options.Passwords.CA,
		"provisioner_password": p.Options.Passwords.Provisioner,
	}

	if p.Options.SSH {
		b["ssh_host_ca_key"] = string(p.SSHHostKey)
		b["ssh_host_ca_key.pub"] = string(p.SSHHostPublicKey)
		b["ssh_user_ca_key"] = string(p.SSHUserKey)
		b["ssh_user_ca_key.pub"] = string(p.SSHUserPublicKey)
		b["ssh_host_password"] = p.Options.Passwords.SSHHost
		b["ssh_user_password"] = p.Options.Passwords.SSHUser
	}

	return b
}
