package core

import (
	fs "github.com/adnoctem/gopskit/pkg/fsi"
	"github.com/adnoctem/gopskit/pkg/kube"
	"github.com/adnoctem/gopskit/pkg/log"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/adnoctem/gopskit/pkg/stamp"
)

// API is the common central application type embedded by most, if not
// all gopskit applications. It merely saves us some writing for commonly
// required API's
type API struct {
	// Name is the name of the application this object is instanced for
	Name string

	// Exec represents a command-line executor to quickly new start processes
	// and closely evaluate their output as well as potential errors
	Exec *proc.Executor

	// Kube is a Kubernetes Client capable of performing all Kubernetes
	// operations, initialized from the same sources as `kubectl`
	Kube *kube.Client

	// Platform is a utility object performing OS-specific lookups and other
	// data gathering tasks
	Paths *fs.PlatformPaths

	// Log is a wrapper object around Uber's `zap` logger
	Log *log.Logger

	// KV is the embedded BadgerDB database wrapper which we us to persist
	// Vault tokens, unseal keys and recovery keys, e.g. after Vault initialization.
	// These values may or may not be made accessible via the CLIs via a subcommand.
	// KV *kv.Database

	// Stamp is build-time information that is linked into the final executable
	// by LD. Our Bazel builds stamps builds via LD using the 'x_defs' attribute
	// on the 'go_library' rule
	Stamp *stamp.Stamps
}
