# gopskit-bridge

The in-cluster half of the gopskit bridge: a small SOCKS5 server through which `ssolo`, `waltr` and
`steppa` reach services inside a Kubernetes cluster by their real names - for example
`https://keycloak.keycloak.svc:8443` or `https://vault-0.vault-internal.vault.svc:8200` - from a
machine outside it.

You don't run this binary yourself. The CLIs deploy it with `proxy setup`, and every command that
needs it opens its own short-lived port-forward to it.

## Why it exists

Before the bridge, the CLIs port-forwarded to an application Pod on a fixed local port and talked
to `https://127.0.0.1:7150`. That allowed one service at a time, could never verify TLS because
`127.0.0.1` matches no certificate, and depended on finding the right Pod by label.

Through the bridge, a CLI asks for the service's real name, the cluster's DNS resolves it, and TLS
is verified against that name. Any number of services are reachable through one tunnel.

## Using it

```sh
# once per cluster - any of the CLIs can do this
ssolo proxy setup

# optionally with its own NetworkPolicy, limited to the namespaces it needs
ssolo proxy setup --network-policy --allow-namespace keycloak,vault

# is it running and reachable, and can it reach a service?
ssolo proxy check --target keycloak.keycloak.svc:8443

# remove it again
ssolo proxy cleanup
```

Commands then reach cluster-internal URLs through it automatically: with `--via auto`, the default,
anything under `.svc` or `.cluster.local` goes through the bridge and everything else is reached
directly. `--via bridge` and `--via direct` force one or the other. A command that needs the bridge
when it isn't running fails and says to run `proxy setup`.

| Flag                  | Command | Meaning                                                                     |
| --------------------- | ------- | --------------------------------------------------------------------------- |
| `--bridge-namespace`  | all     | where the bridge runs (default `gopskit-bridge`)                            |
| `--image`             | `setup` | the image to run (default: the release matching the CLI's own version)      |
| `--network-policy`    | `setup` | also deploy the bridge's NetworkPolicy; running setup without it removes it |
| `--allow-namespace`   | `setup` | namespaces the policy lets the bridge reach (default: all)                  |
| `--wait`, `--timeout` | `setup` | wait for the bridge to become ready (default: yes, 2 minutes)               |
| `--target`            | `check` | a `host:port` to connect to through the bridge (repeatable)                 |

## Security model

- **Loopback only.** The server listens on `127.0.0.1` and `::1` inside its Pod, so nothing in the
  cluster network can connect to it. The only way in is a port-forward, which makes Kubernetes RBAC
  - permission to port-forward to Pods in the bridge's namespace - the access control.
- **Hardened Pod.** It satisfies the "restricted" Pod Security Standard: non-root UID 65532,
  read-only root filesystem, all capabilities dropped, no privilege escalation, RuntimeDefault
  seccomp, and no ServiceAccount token. The namespace `setup` creates enforces that standard.
- **Optional NetworkPolicy.** With `--network-policy`, the bridge gets a policy denying all ingress
  and allowing egress only to DNS and the namespaces given with `--allow-namespace` (every namespace
  in the cluster if none) - never to anything outside the cluster.
- **Leaves foreign objects alone.** `setup` refuses to overwrite same-named resources it didn't
  create, and `cleanup` deletes the namespace only if `setup` created it.

These properties are tested against a real k3s cluster, including that the bridge refuses
connections to its own Pod IP and that port-forwards keep working while the NetworkPolicy denies
all ingress.

## The image

`ghcr.io/adnoctem/gopskit-bridge`, published for amd64 and arm64 with every gopskit release and
tagged with the release version and `latest`. It holds nothing but the static binary.

To build it yourself, or for an air-gapped cluster:

```sh
# multi-arch image index
bazel build //cmd/gopskit-bridge:image

# load the amd64 image into the local Docker engine as gopskit-bridge:dev
bazel run //cmd/gopskit-bridge:load

# then point setup at your registry's copy
ssolo proxy setup --image registry.example.org/gopskit-bridge:1.2.0
```

## The binary

```text
gopskit-bridge [--port 1080]                  serve SOCKS5 on 127.0.0.1 and ::1
gopskit-bridge probe [--port 1080] [--timeout 2s]   exit 0 if the server answers
```

It implements the part of SOCKS5 (RFC 1928) the CLIs use: `CONNECT`, without authentication, to
IPv4, IPv6 and domain-name destinations, with domain names resolved by the server. `probe` exists
for the kubelet: a TCP probe targets the Pod IP and can't reach a loopback-only listener, and the
image has no shell to run anything else.
