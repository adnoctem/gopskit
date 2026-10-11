package bridge

import (
	"maps"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

const (
	// Name names every resource the bridge consists of.
	Name = "gopskit-bridge"

	// DefaultNamespace is where `proxy setup` deploys the bridge unless told otherwise.
	DefaultNamespace = "gopskit-bridge"

	// Port is the loopback port gopskit-bridge listens on inside its Pod.
	Port = 1080

	// uid is the unprivileged user the image's files belong to.
	uid = 65532

	// LabelManagedBy marks resources created by `proxy setup`. Cleanup only ever deletes
	// resources carrying it, and only deletes the namespace when setup created that too.
	LabelManagedBy = "app.kubernetes.io/managed-by"
	managedBy      = "gopskit"
)

// selector identifies the bridge Pods.
var selector = map[string]string{
	"app.kubernetes.io/name":     Name,
	"app.kubernetes.io/instance": Name,
}

// Labels returns the labels every bridge resource carries.
func Labels() map[string]string {
	l := map[string]string{LabelManagedBy: managedBy, "app.kubernetes.io/component": "bridge"}
	maps.Copy(l, selector)

	return l
}

// Namespace is the namespace `proxy setup` creates when it doesn't exist yet. It enforces the
// "restricted" Pod Security Standard, which the bridge Pod satisfies.
func Namespace(name string) *corev1.Namespace {
	labels := Labels()
	labels["pod-security.kubernetes.io/enforce"] = "restricted"

	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
	}
}

// ServiceAccount is the bridge's identity. The bridge never talks to the Kubernetes API, so the
// account has no roles and its token is never mounted.
func ServiceAccount(namespace string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta:                   metav1.ObjectMeta{Name: Name, Namespace: namespace, Labels: Labels()},
		AutomountServiceAccountToken: ptr.To(false),
	}
}

// Deployment runs a single bridge Pod hardened to the "restricted" Pod Security Standard.
func Deployment(namespace, image string) *appsv1.Deployment {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			// a tcpSocket probe targets the Pod IP, which the loopback-only listener ignores
			Exec: &corev1.ExecAction{Command: []string{"/gopskit-bridge", "probe"}},
		},
		PeriodSeconds:  10,
		TimeoutSeconds: 3,
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: Name, Namespace: namespace, Labels: Labels()},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: Labels()},
				Spec: corev1.PodSpec{
					ServiceAccountName:           Name,
					AutomountServiceAccountToken: ptr.To(false),
					EnableServiceLinks:           ptr.To(false),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   ptr.To(true),
						RunAsUser:      ptr.To[int64](uid),
						RunAsGroup:     ptr.To[int64](uid),
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Containers: []corev1.Container{{
						Name:            Name,
						Image:           image,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Args:            []string{"--port", strconv.Itoa(Port)},
						Ports: []corev1.ContainerPort{{
							Name:          "socks",
							ContainerPort: Port,
							Protocol:      corev1.ProtocolTCP,
						}},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							ReadOnlyRootFilesystem:   ptr.To(true),
							Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("16Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
						},
						ReadinessProbe: probe,
						LivenessProbe:  probe,
					}},
				},
			},
		},
	}
}

// NetworkPolicy restricts the bridge Pod to what it needs. Ingress is denied outright: the bridge
// is reached through a port-forward into its loopback interface, which no NetworkPolicy governs.
// Egress is allowed to DNS, and to the Pods of allowNamespaces - or of every namespace when that
// is empty. Destinations outside the cluster are never allowed.
func NetworkPolicy(namespace string, allowNamespaces []string) *networkingv1.NetworkPolicy {
	dns := networkingv1.NetworkPolicyEgressRule{
		// by port rather than by the DNS Pods' labels, so NodeLocal DNSCache and other
		// non-standard resolvers keep working
		Ports: []networkingv1.NetworkPolicyPort{
			{Protocol: ptr.To(corev1.ProtocolUDP), Port: ptr.To(intstr.FromInt32(53))},
			{Protocol: ptr.To(corev1.ProtocolTCP), Port: ptr.To(intstr.FromInt32(53))},
		},
	}

	targets := networkingv1.NetworkPolicyEgressRule{}
	if len(allowNamespaces) == 0 {
		targets.To = []networkingv1.NetworkPolicyPeer{{NamespaceSelector: &metav1.LabelSelector{}}}
	} else {
		targets.To = []networkingv1.NetworkPolicyPeer{{
			NamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      corev1.LabelMetadataName,
					Operator: metav1.LabelSelectorOpIn,
					Values:   allowNamespaces,
				}},
			},
		}}
	}

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: Name, Namespace: namespace, Labels: Labels()},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: selector},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
			Ingress:     []networkingv1.NetworkPolicyIngressRule{},
			Egress:      []networkingv1.NetworkPolicyEgressRule{dns, targets},
		},
	}
}
