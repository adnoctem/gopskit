package kv

import (
	"context"
	"fmt"
	"sync"

	"github.com/dgraph-io/badger/v4"
)

const (
	DefaultKVName       = "store"
	DefaultNamespace    = "default"
	DefaultDiscardRatio = 0.7
)

var (
	DefaultPath       = fmt.Sprintf("/tmp/%s", DefaultKVName)
	DefaultNamespaces = []string{DefaultNamespace}
)

// Database is the central type for the persisted Key-Value database
type Database struct {
	// kv is the underlying BadgerDB key-value database, which we lock to run in an
	// on-disk persistent mode which our CLI's can utilize.
	kv *badger.DB

	// options refers to a copy of the badger.Options with which the current instance
	// badger.DB was initialized. This mainly provided for later reference rather than
	// (re)-configuration.
	options badger.Options

	// path is the filesystem path the database is persisted to
	path string

	// namespaces are the namespaces managed by the Database. These have to be
	// registered with the database at initialization using WithNamespace. By default,
	// the database uses a single namespace "default".
	namespaces []string

	// currentNamespace is the currently used namespace to write keys to. Since every
	// database operation like Get, Set, etc. needs a namespace we use the "default"
	// namespace if no other is provided.
	currentNamespace string

	// discardRatio is the BadgerDB configuration value used to determine when a
	// file can be rewritten. By default, a DefaultDiscardRatio of 0.7 will be used.
	discardRatio float64

	// ctx represents the embedded database context to cancel operations
	ctx context.Context

	// cancel is the context.CancelFunc associated with the embedded context
	cancel context.CancelFunc

	// lock is a Mutex which ensures that only one goroutine may write or reads
	// from the database at a time to prevent corruptions
	lock sync.Mutex
}

type Store interface {
	// Get retrieves the value of a given key within the current namespace from
	// the BadgerDB database. If a different namespace is to be checked
	// SetNamespace must be used beforehand.
	Get(key string) (value []byte, err error)

	// Set updates or sets the value for a given key within the current namespace.
	// It takes the value as a byte-slice instead of a string or a different type
	// to offer more versatility. If a different namespace is to be checked
	// SetNamespace must be used beforehand.
	Set(key string, value []byte) error

	// Has checks whether a database key exists within the current namespace. If
	// a different namespace is to be checked SetNamespace must be used beforehand.
	Has(key string) (contains bool, err error)

	// Namespaces lists all currently registered namespaces in the Database
	Namespaces() []string

	// Delete removes a key within the current namespace along with all of its data.
	// This is a non-recoverable operation. If a different namespace is to be checked
	// SetNamespace must be used beforehand.
	Delete(key string) error

	// Close closes the underlying connection to the BadgerDB database. This would
	// preferably be used with a `defer` statement in the main function since the
	// database may be need for the entire duration of the program.
	Close() error

	// Path returns the filesystem path the database was initialized for. This value
	// equal to the options.Dir and options.ValueDir fields.
	Path() string

	// Config returns the entire configuration with which the BadgerDB database was
	// initialized.
	Config() badger.Options
}

// Opt is a configuration option for the newly initialized Key-Value database
type Opt func(k *Database)
