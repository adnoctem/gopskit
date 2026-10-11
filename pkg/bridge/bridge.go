// Package bridge manages the gopskit bridge: a SOCKS5 server deployed into a Kubernetes cluster,
// through which the gopskit CLIs reach cluster-internal services such as Keycloak and Vault by
// their real service names.
//
// The bridge is set up once, with `<cli> proxy setup`, and left running; every command that needs
// it opens its own short-lived port-forward to it with Connect. Commands therefore fail fast when
// the bridge isn't there, instead of creating and tearing it down on every invocation.
package bridge

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/adnoctem/gopskit/pkg/bridge/socks5"
	"github.com/adnoctem/gopskit/pkg/kube"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// ImageRepository is where release builds publish the gopskit-bridge image.
const ImageRepository = "ghcr.io/adnoctem/gopskit-bridge"

// ErrNotRunning is returned by Connect when no ready bridge Pod exists.
var ErrNotRunning = errors.New("the gopskit bridge is not running")

var releaseVersion = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// DefaultImage returns the bridge image matching a CLI's own version, so the two halves of the
// bridge always come from the same release. Development builds, which carry a `git describe`
// version or none at all, fall back to the latest release.
func DefaultImage(cliVersion string) string {
	if releaseVersion.MatchString(cliVersion) {
		return ImageRepository + ":" + cliVersion
	}

	return ImageRepository + ":latest"
}

// Config describes a bridge deployment.
type Config struct {
	// Namespace is where the bridge runs. It is created, and later deleted, by the bridge if it
	// doesn't exist yet; an existing namespace is used as-is and left in place by Cleanup.
	Namespace string

	// Image is the gopskit-bridge image to run.
	Image string

	// NetworkPolicy additionally deploys the bridge's own NetworkPolicy. Disabling it removes a
	// policy a previous setup created.
	NetworkPolicy bool

	// AllowNamespaces limits the NetworkPolicy's egress to these namespaces. Empty allows every
	// namespace in the cluster.
	AllowNamespaces []string
}

// Setup creates or updates the bridge. It is idempotent, and reports what it changed through
// logf, which may be nil.
func Setup(ctx context.Context, cs kubernetes.Interface, cfg Config, logf func(string, ...any)) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	if cfg.Namespace == "" || cfg.Image == "" {
		return errors.New("bridge setup requires a namespace and an image")
	}

	_, err := cs.CoreV1().Namespaces().Get(ctx, cfg.Namespace, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		if _, err := cs.CoreV1().Namespaces().Create(ctx, Namespace(cfg.Namespace), metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("could not create namespace %s: %w", cfg.Namespace, err)
		}
		logf("created namespace %s", cfg.Namespace)
	case err != nil:
		return fmt.Errorf("could not look up namespace %s: %w", cfg.Namespace, err)
	}

	if err := upsertServiceAccount(ctx, cs, cfg.Namespace, logf); err != nil {
		return err
	}

	if err := upsertDeployment(ctx, cs, cfg, logf); err != nil {
		return err
	}

	if cfg.NetworkPolicy {
		return upsertNetworkPolicy(ctx, cs, cfg, logf)
	}

	return deleteOwned(ctx, cs.NetworkingV1().NetworkPolicies(cfg.Namespace), "NetworkPolicy", logf)
}

// Cleanup removes the bridge. A namespace Setup created is deleted along with everything in it;
// in a namespace that existed before, only the bridge's own resources are.
func Cleanup(ctx context.Context, cs kubernetes.Interface, namespace string, logf func(string, ...any)) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	ns, err := cs.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		logf("namespace %s does not exist, nothing to clean up", namespace)
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not look up namespace %s: %w", namespace, err)
	}

	if owned(ns.Labels) {
		if err := cs.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("could not delete namespace %s: %w", namespace, err)
		}
		logf("deleted namespace %s", namespace)
		return nil
	}

	if err := deleteOwned(ctx, cs.NetworkingV1().NetworkPolicies(namespace), "NetworkPolicy", logf); err != nil {
		return err
	}
	if err := deleteOwned(ctx, cs.AppsV1().Deployments(namespace), "Deployment", logf); err != nil {
		return err
	}

	return deleteOwned(ctx, cs.CoreV1().ServiceAccounts(namespace), "ServiceAccount", logf)
}

// Status summarises a bridge deployment.
type Status struct {
	Image         string
	ReadyReplicas int32
	NetworkPolicy bool
}

// GetStatus reports on the bridge in namespace, or ErrNotRunning when it isn't deployed there.
func GetStatus(ctx context.Context, cs kubernetes.Interface, namespace string) (*Status, error) {
	dep, err := cs.AppsV1().Deployments(namespace).Get(ctx, Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("%w: no %s Deployment in namespace %s", ErrNotRunning, Name, namespace)
	}
	if err != nil {
		return nil, err
	}

	st := &Status{ReadyReplicas: dep.Status.ReadyReplicas}
	if len(dep.Spec.Template.Spec.Containers) > 0 {
		st.Image = dep.Spec.Template.Spec.Containers[0].Image
	}

	_, err = cs.NetworkingV1().NetworkPolicies(namespace).Get(ctx, Name, metav1.GetOptions{})
	switch {
	case err == nil:
		st.NetworkPolicy = true
	case !apierrors.IsNotFound(err):
		return nil, err
	}

	return st, nil
}

// WaitReady blocks until the bridge Deployment has rolled out and has a ready Pod.
func WaitReady(ctx context.Context, cs kubernetes.Interface, namespace string) error {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	for {
		dep, err := cs.AppsV1().Deployments(namespace).Get(ctx, Name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if err == nil && rolledOut(dep) {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("bridge did not become ready: %w", ctx.Err())
		case <-tick.C:
		}
	}
}

func rolledOut(dep *appsv1.Deployment) bool {
	want := int32(1)
	if dep.Spec.Replicas != nil {
		want = *dep.Spec.Replicas
	}

	s := dep.Status
	return s.ObservedGeneration >= dep.Generation &&
		s.UpdatedReplicas == want && s.ReadyReplicas >= want && s.Replicas == want
}

// Tunnel is an open connection to the bridge: a port-forward to the bridge Pod's SOCKS5 port.
type Tunnel struct {
	forward *kube.Forward
}

// Connect opens a tunnel to a ready bridge Pod in namespace and verifies the bridge answers.
func Connect(ctx context.Context, kc *kube.Client, namespace string) (*Tunnel, error) {
	pods, err := kc.Client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(selector).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("could not look for bridge Pods in namespace %s: %w", namespace, err)
	}

	var pod *corev1.Pod
	for i := range pods.Items {
		if podReady(&pods.Items[i]) {
			pod = &pods.Items[i]
			break
		}
	}
	if pod == nil {
		return nil, fmt.Errorf("%w: no ready bridge Pod in namespace %s", ErrNotRunning, namespace)
	}

	fwd, err := kc.Forward(ctx, *pod, Port)
	if err != nil {
		return nil, fmt.Errorf("could not port-forward to bridge Pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}

	t := &Tunnel{forward: fwd}
	if err := socks5.Greet(ctx, t.Addr()); err != nil {
		_ = t.Close()
		return nil, fmt.Errorf("bridge Pod %s/%s does not answer: %w", pod.Namespace, pod.Name, err)
	}

	return t, nil
}

func podReady(pod *corev1.Pod) bool {
	if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
		return false
	}

	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}

	return false
}

// Addr is the local address of the bridge's SOCKS5 server.
func (t *Tunnel) Addr() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(int(t.forward.LocalPort)))
}

// ProxyURL is the bridge as an HTTP proxy URL. The scheme is socks5, under which Go's HTTP client
// sends host names unresolved, so the cluster's DNS resolves them.
func (t *Tunnel) ProxyURL() *url.URL {
	return &url.URL{Scheme: "socks5", Host: t.Addr()}
}

// Transport returns an HTTP transport sending every request through the bridge.
func (t *Tunnel) Transport(tlsConfig *tls.Config) *http.Transport {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = http.ProxyURL(t.ProxyURL())
	tr.TLSClientConfig = tlsConfig

	return tr
}

// Dial opens a TCP connection to target ("host:port") through the bridge.
func (t *Tunnel) Dial(ctx context.Context, target string) (net.Conn, error) {
	return socks5.Dial(ctx, t.Addr(), target)
}

// Close closes the tunnel.
func (t *Tunnel) Close() error {
	return t.forward.Close()
}

// InCluster reports whether host is a cluster-internal service name, which only the bridge can
// resolve: anything under .svc or .cluster.local.
func InCluster(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")

	return strings.HasSuffix(host, ".svc") || strings.HasSuffix(host, ".cluster.local")
}

// ---- upserts ----

func upsertServiceAccount(ctx context.Context, cs kubernetes.Interface, namespace string, logf func(string, ...any)) error {
	want := ServiceAccount(namespace)
	client := cs.CoreV1().ServiceAccounts(namespace)

	have, err := client.Get(ctx, Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := client.Create(ctx, want, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("could not create ServiceAccount: %w", err)
		}
		logf("created ServiceAccount %s/%s", namespace, Name)
		return nil
	}
	if err != nil {
		return err
	}
	if err := claimable(have.ObjectMeta, "ServiceAccount"); err != nil {
		return err
	}

	have.Labels = want.Labels
	have.AutomountServiceAccountToken = want.AutomountServiceAccountToken
	if _, err := client.Update(ctx, have, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("could not update ServiceAccount: %w", err)
	}
	logf("updated ServiceAccount %s/%s", namespace, Name)
	return nil
}

func upsertDeployment(ctx context.Context, cs kubernetes.Interface, cfg Config, logf func(string, ...any)) error {
	want := Deployment(cfg.Namespace, cfg.Image)
	client := cs.AppsV1().Deployments(cfg.Namespace)

	have, err := client.Get(ctx, Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := client.Create(ctx, want, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("could not create Deployment: %w", err)
		}
		logf("created Deployment %s/%s running %s", cfg.Namespace, Name, cfg.Image)
		return nil
	}
	if err != nil {
		return err
	}
	if err := claimable(have.ObjectMeta, "Deployment"); err != nil {
		return err
	}

	have.Labels = want.Labels
	have.Spec = want.Spec
	if _, err := client.Update(ctx, have, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("could not update Deployment: %w", err)
	}
	logf("updated Deployment %s/%s running %s", cfg.Namespace, Name, cfg.Image)
	return nil
}

func upsertNetworkPolicy(ctx context.Context, cs kubernetes.Interface, cfg Config, logf func(string, ...any)) error {
	want := NetworkPolicy(cfg.Namespace, cfg.AllowNamespaces)
	client := cs.NetworkingV1().NetworkPolicies(cfg.Namespace)

	have, err := client.Get(ctx, Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := client.Create(ctx, want, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("could not create NetworkPolicy: %w", err)
		}
		logf("created NetworkPolicy %s/%s", cfg.Namespace, Name)
		return nil
	}
	if err != nil {
		return err
	}
	if err := claimable(have.ObjectMeta, "NetworkPolicy"); err != nil {
		return err
	}

	have.Labels = want.Labels
	have.Spec = want.Spec
	if _, err := client.Update(ctx, have, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("could not update NetworkPolicy: %w", err)
	}
	logf("updated NetworkPolicy %s/%s", cfg.Namespace, Name)
	return nil
}

// claimable refuses to take over a same-named resource the bridge didn't create.
func claimable(meta metav1.ObjectMeta, kind string) error {
	if owned(meta.Labels) {
		return nil
	}

	return fmt.Errorf("%s %s/%s exists but was not created by the gopskit bridge; refusing to overwrite it",
		kind, meta.Namespace, meta.Name)
}

func owned(l map[string]string) bool {
	if l[LabelManagedBy] != managedBy {
		return false
	}

	for k, v := range selector {
		if l[k] != v {
			return false
		}
	}

	return true
}

// getDeleter is the subset of a typed client deleteOwned needs.
type getDeleter[T metav1.Object] interface {
	Get(ctx context.Context, name string, opts metav1.GetOptions) (T, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
}

// deleteOwned deletes the bridge's resource of one kind, if it exists and the bridge created it.
func deleteOwned[T metav1.Object](ctx context.Context, client getDeleter[T], kind string, logf func(string, ...any)) error {
	obj, err := client.Get(ctx, Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if !owned(obj.GetLabels()) {
		logf("leaving %s %s/%s in place: it was not created by the gopskit bridge", kind, obj.GetNamespace(), Name)
		return nil
	}

	if err := client.Delete(ctx, Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("could not delete %s %s/%s: %w", kind, obj.GetNamespace(), Name, err)
	}
	logf("deleted %s %s/%s", kind, obj.GetNamespace(), Name)
	return nil
}
