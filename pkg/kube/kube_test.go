package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestClient() *Client {
	return &Client{Client: fake.NewSimpleClientset()}
}

func TestNamespacesGet(t *testing.T) {
	asrt := assert.New(t)

	c := newTestClient()
	_, err := c.Client.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "team-a"},
	}, metav1.CreateOptions{})
	asrt.NoError(err)

	got, err := c.Namespaces(metav1.ListOptions{})
	asrt.NoError(err)
	asrt.Len(got, 1)
	asrt.Equal("team-a", got[0].Name)
}

func TestPodsGet(t *testing.T) {
	asrt := assert.New(t)

	c := newTestClient()
	_, err := c.Client.CoreV1().Pods("ns").Create(t.Context(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-a"},
	}, metav1.CreateOptions{})
	asrt.NoError(err)

	got, err := c.Pods("ns", metav1.ListOptions{})
	asrt.NoError(err)
	asrt.Len(got, 1)
	asrt.Equal("pod-a", got[0].Name)

	// a different namespace must not see it
	got, err = c.Pods("other", metav1.ListOptions{})
	asrt.NoError(err)
	asrt.Empty(got)
}

func TestServiceGetAndList(t *testing.T) {
	asrt := assert.New(t)

	c := newTestClient()
	asrt.NoError(c.CreateService("ns", &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-a"}}, metav1.CreateOptions{}))

	got, err := c.Service("ns", "svc-a", metav1.GetOptions{})
	asrt.NoError(err)
	asrt.Equal("svc-a", got.Name)

	_, err = c.Service("ns", "missing", metav1.GetOptions{})
	asrt.Error(err)

	list, err := c.Services("ns", metav1.ListOptions{})
	asrt.NoError(err)
	asrt.Len(list, 1)
}

func TestConfigMapGetAndList(t *testing.T) {
	asrt := assert.New(t)

	c := newTestClient()
	_, err := c.Client.CoreV1().ConfigMaps("ns").Create(t.Context(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-a"},
		Data:       map[string]string{"k": "v"},
	}, metav1.CreateOptions{})
	asrt.NoError(err)

	got, err := c.ConfigMap("ns", "cm-a", metav1.GetOptions{})
	asrt.NoError(err)
	asrt.Equal("v", got.Data["k"])

	list, err := c.ConfigMaps("ns", metav1.ListOptions{})
	asrt.NoError(err)
	asrt.Len(list, 1)
}

func TestSecretGetAndList(t *testing.T) {
	asrt := assert.New(t)

	c := newTestClient()
	asrt.NoError(c.CreateSecret("ns", &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sec-a"}}, metav1.CreateOptions{}))

	got, err := c.Secret("ns", "sec-a", metav1.GetOptions{})
	asrt.NoError(err)
	asrt.Equal("sec-a", got.Name)

	list, err := c.Secrets("ns", metav1.ListOptions{})
	asrt.NoError(err)
	asrt.Len(list, 1)
}

func TestIngressesList(t *testing.T) {
	asrt := assert.New(t)

	c := newTestClient()
	asrt.NoError(c.CreateIngress("ns", &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "ing-a"}}, metav1.CreateOptions{}))

	list, err := c.Ingresses("ns", metav1.ListOptions{})
	asrt.NoError(err)
	asrt.Len(list, 1)
	asrt.Equal("ing-a", list[0].Name)
}

func TestStorageClassGet(t *testing.T) {
	asrt := assert.New(t)

	c := newTestClient()
	asrt.NoError(c.CreateStorageClass(&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "sc-a"}}, metav1.CreateOptions{}))

	got, err := c.StorageClass("sc-a", metav1.GetOptions{})
	asrt.NoError(err)
	asrt.Equal("sc-a", got.Name)
}

func TestCreatePod(t *testing.T) {
	asrt := assert.New(t)

	c := newTestClient()
	asrt.NoError(c.CreatePod("ns", &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-a"}}, metav1.CreateOptions{}))

	got, err := c.Client.CoreV1().Pods("ns").Get(t.Context(), "pod-a", metav1.GetOptions{})
	asrt.NoError(err)
	asrt.Equal("pod-a", got.Name)
}

func TestCreateDeployment(t *testing.T) {
	asrt := assert.New(t)

	c := newTestClient()
	asrt.NoError(c.CreateDeployment("ns", &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "dep-a"}}, metav1.CreateOptions{}))

	got, err := c.Client.AppsV1().Deployments("ns").Get(t.Context(), "dep-a", metav1.GetOptions{})
	asrt.NoError(err)
	asrt.Equal("dep-a", got.Name)
}
