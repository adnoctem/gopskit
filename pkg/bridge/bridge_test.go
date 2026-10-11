package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const testImage = ImageRepository + ":1.2.3"

func TestDefaultImage(t *testing.T) {
	asrt := assert.New(t)

	asrt.Equal(ImageRepository+":1.2.3", DefaultImage("1.2.3"))
	asrt.Equal(ImageRepository+":latest", DefaultImage("1.1.2-6-g92f8b00-dirty"), "a development build has no matching image")
	asrt.Equal(ImageRepository+":latest", DefaultImage(""))
	asrt.Equal(ImageRepository+":latest", DefaultImage("v1.2.3"), "release versions are stamped without the v")
}

func TestInCluster(t *testing.T) {
	asrt := assert.New(t)

	asrt.True(InCluster("keycloak.keycloak.svc"))
	asrt.True(InCluster("vault-0.vault-internal.vault.svc.cluster.local"))
	asrt.True(InCluster("Vault.Vault.SVC."))
	asrt.False(InCluster("sso.example.org"))
	asrt.False(InCluster("127.0.0.1"))
	asrt.False(InCluster("svc"))
}

func TestSetupCreatesHardenedBridge(t *testing.T) {
	asrt := assert.New(t)
	ctx := context.Background()
	cs := fake.NewClientset()

	require.NoError(t, Setup(ctx, cs, Config{Namespace: DefaultNamespace, Image: testImage}, nil))

	ns, err := cs.CoreV1().Namespaces().Get(ctx, DefaultNamespace, metav1.GetOptions{})
	require.NoError(t, err)
	asrt.Equal("restricted", ns.Labels["pod-security.kubernetes.io/enforce"])
	asrt.True(owned(ns.Labels), "a namespace setup created must be marked as the bridge's")

	sa, err := cs.CoreV1().ServiceAccounts(DefaultNamespace).Get(ctx, Name, metav1.GetOptions{})
	require.NoError(t, err)
	asrt.False(*sa.AutomountServiceAccountToken)

	dep, err := cs.AppsV1().Deployments(DefaultNamespace).Get(ctx, Name, metav1.GetOptions{})
	require.NoError(t, err)

	pod := dep.Spec.Template.Spec
	asrt.False(*pod.AutomountServiceAccountToken)
	asrt.True(*pod.SecurityContext.RunAsNonRoot)
	asrt.Equal(corev1.SeccompProfileTypeRuntimeDefault, pod.SecurityContext.SeccompProfile.Type)

	require.Len(t, pod.Containers, 1)
	c := pod.Containers[0]
	asrt.Equal(testImage, c.Image)
	asrt.False(*c.SecurityContext.AllowPrivilegeEscalation)
	asrt.True(*c.SecurityContext.ReadOnlyRootFilesystem)
	asrt.Equal([]corev1.Capability{"ALL"}, c.SecurityContext.Capabilities.Drop)
	asrt.Equal([]string{"/gopskit-bridge", "probe"}, c.ReadinessProbe.Exec.Command)
	asrt.Equal(dep.Spec.Selector.MatchLabels, selector)

	_, err = cs.NetworkingV1().NetworkPolicies(DefaultNamespace).Get(ctx, Name, metav1.GetOptions{})
	asrt.True(apierrors.IsNotFound(err), "the NetworkPolicy is opt-in")
}

func TestSetupIsIdempotentAndUpdatesImage(t *testing.T) {
	asrt := assert.New(t)
	ctx := context.Background()
	cs := fake.NewClientset()

	cfg := Config{Namespace: DefaultNamespace, Image: testImage}
	require.NoError(t, Setup(ctx, cs, cfg, nil))

	cfg.Image = ImageRepository + ":1.2.4"
	require.NoError(t, Setup(ctx, cs, cfg, nil))

	deps, err := cs.AppsV1().Deployments(DefaultNamespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, deps.Items, 1)
	asrt.Equal(cfg.Image, deps.Items[0].Spec.Template.Spec.Containers[0].Image)
}

func TestSetupNetworkPolicyToggle(t *testing.T) {
	asrt := assert.New(t)
	ctx := context.Background()
	cs := fake.NewClientset()

	cfg := Config{Namespace: DefaultNamespace, Image: testImage, NetworkPolicy: true, AllowNamespaces: []string{"keycloak", "vault"}}
	require.NoError(t, Setup(ctx, cs, cfg, nil))

	np, err := cs.NetworkingV1().NetworkPolicies(DefaultNamespace).Get(ctx, Name, metav1.GetOptions{})
	require.NoError(t, err)
	asrt.Equal([]networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}, np.Spec.PolicyTypes)
	asrt.Empty(np.Spec.Ingress, "nothing in the cluster network may reach the bridge")
	asrt.Equal([]string{"keycloak", "vault"}, np.Spec.Egress[1].To[0].NamespaceSelector.MatchExpressions[0].Values)

	cfg.NetworkPolicy = false
	require.NoError(t, Setup(ctx, cs, cfg, nil))

	_, err = cs.NetworkingV1().NetworkPolicies(DefaultNamespace).Get(ctx, Name, metav1.GetOptions{})
	asrt.True(apierrors.IsNotFound(err), "disabling the policy must remove the one setup created")
}

func TestNetworkPolicyWithoutAllowListAllowsWholeCluster(t *testing.T) {
	asrt := assert.New(t)

	np := NetworkPolicy("ns", nil)
	require.Len(t, np.Spec.Egress, 2)

	peer := np.Spec.Egress[1].To[0]
	asrt.NotNil(peer.NamespaceSelector)
	asrt.Empty(peer.NamespaceSelector.MatchLabels)
	asrt.Empty(peer.NamespaceSelector.MatchExpressions)
	asrt.Nil(peer.IPBlock, "destinations outside the cluster must never be allowed")
}

func TestSetupRefusesForeignResources(t *testing.T) {
	asrt := assert.New(t)
	ctx := context.Background()

	foreign := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: Name, Namespace: "tools"}}
	cs := fake.NewClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tools"}}, foreign)

	err := Setup(ctx, cs, Config{Namespace: "tools", Image: testImage}, nil)
	asrt.ErrorContains(err, "not created by the gopskit bridge")
}

func TestCleanupDeletesNamespaceSetupCreated(t *testing.T) {
	asrt := assert.New(t)
	ctx := context.Background()
	cs := fake.NewClientset()

	require.NoError(t, Setup(ctx, cs, Config{Namespace: DefaultNamespace, Image: testImage, NetworkPolicy: true}, nil))
	require.NoError(t, Cleanup(ctx, cs, DefaultNamespace, nil))

	_, err := cs.CoreV1().Namespaces().Get(ctx, DefaultNamespace, metav1.GetOptions{})
	asrt.True(apierrors.IsNotFound(err))
}

func TestCleanupKeepsPreexistingNamespace(t *testing.T) {
	asrt := assert.New(t)
	ctx := context.Background()

	unrelated := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "someone-else", Namespace: "tools"}}
	cs := fake.NewClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tools"}}, unrelated)

	require.NoError(t, Setup(ctx, cs, Config{Namespace: "tools", Image: testImage, NetworkPolicy: true}, nil))
	require.NoError(t, Cleanup(ctx, cs, "tools", nil))

	_, err := cs.CoreV1().Namespaces().Get(ctx, "tools", metav1.GetOptions{})
	asrt.NoError(err, "a namespace the bridge didn't create must survive cleanup")

	_, err = cs.CoreV1().ServiceAccounts("tools").Get(ctx, "someone-else", metav1.GetOptions{})
	asrt.NoError(err)

	_, err = cs.AppsV1().Deployments("tools").Get(ctx, Name, metav1.GetOptions{})
	asrt.True(apierrors.IsNotFound(err))
	_, err = cs.CoreV1().ServiceAccounts("tools").Get(ctx, Name, metav1.GetOptions{})
	asrt.True(apierrors.IsNotFound(err))
	_, err = cs.NetworkingV1().NetworkPolicies("tools").Get(ctx, Name, metav1.GetOptions{})
	asrt.True(apierrors.IsNotFound(err))
}

func TestCleanupMissingNamespaceIsNoop(t *testing.T) {
	assert.NoError(t, Cleanup(context.Background(), fake.NewClientset(), "absent", nil))
}

func TestGetStatus(t *testing.T) {
	asrt := assert.New(t)
	ctx := context.Background()
	cs := fake.NewClientset()

	_, err := GetStatus(ctx, cs, DefaultNamespace)
	asrt.ErrorIs(err, ErrNotRunning)

	require.NoError(t, Setup(ctx, cs, Config{Namespace: DefaultNamespace, Image: testImage, NetworkPolicy: true}, nil))

	st, err := GetStatus(ctx, cs, DefaultNamespace)
	require.NoError(t, err)
	asrt.Equal(testImage, st.Image)
	asrt.True(st.NetworkPolicy)
	asrt.Zero(st.ReadyReplicas)
}

func TestWaitReady(t *testing.T) {
	ctx := context.Background()
	cs := fake.NewClientset()
	require.NoError(t, Setup(ctx, cs, Config{Namespace: DefaultNamespace, Image: testImage}, nil))

	short, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	assert.Error(t, WaitReady(short, cs, DefaultNamespace), "a Deployment with no ready Pod is not ready")

	dep, err := cs.AppsV1().Deployments(DefaultNamespace).Get(ctx, Name, metav1.GetOptions{})
	require.NoError(t, err)
	dep.Status = appsv1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1}
	_, err = cs.AppsV1().Deployments(DefaultNamespace).UpdateStatus(ctx, dep, metav1.UpdateOptions{})
	require.NoError(t, err)

	done, cancel2 := context.WithTimeout(ctx, 5*time.Second)
	defer cancel2()
	assert.NoError(t, WaitReady(done, cs, DefaultNamespace))
}

func TestPodReady(t *testing.T) {
	asrt := assert.New(t)

	ready := corev1.Pod{Status: corev1.PodStatus{
		Phase:      corev1.PodRunning,
		Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
	}}
	asrt.True(podReady(&ready))

	notReady := *ready.DeepCopy()
	notReady.Status.Conditions[0].Status = corev1.ConditionFalse
	asrt.False(podReady(&notReady))

	terminating := *ready.DeepCopy()
	terminating.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	asrt.False(podReady(&terminating), "a terminating Pod is about to drop the tunnel")

	pending := *ready.DeepCopy()
	pending.Status.Phase = corev1.PodPending
	asrt.False(podReady(&pending))
}
