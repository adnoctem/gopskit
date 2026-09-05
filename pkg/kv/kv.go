// Package kv implements a persistent Key-Value store based on the open-source BadgerDB. The implementation supports
// namespacing keys and their values as well as using a global (shared) namespace. The entire storage system works
// asynchronously using mutexes to restrict access to a single goroutine at a time.
package kv

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/dgraph-io/badger/v4"
	fs "github.com/fmjstudios/gopskit/pkg/fsi"
)

// New instantiates a new key-value store and configures it with the given Opt
// configuration options
func New(path string, opts ...Opt) (*Database, error) {
	ctx, cancel := context.WithCancel(context.TODO())
	bOpts := badger.DefaultOptions(path)

	db := &Database{
		kv:               nil,
		options:          bOpts,
		path:             path,
		namespaces:       DefaultNamespaces,
		currentNamespace: DefaultNamespaces[0],
		discardRatio:     DefaultDiscardRatio,
		ctx:              ctx,
		cancel:           cancel,
		lock:             sync.Mutex{},
	}

	// (re)-configure
	for _, opt := range opts {
		opt(db)
	}

	// create path if it does not exist
	exists := fs.CheckIfExists(path)
	if !exists {
		err := os.MkdirAll(path, 0755)
		if err != nil {
			defer cancel()
			return nil, err
		}
	}

	// db
	bdb, err := badger.Open(db.options)
	if err != nil {
		return nil, fmt.Errorf("could not open badger database: %w", err)
	}
	db.kv = bdb

	return db, nil
}

// SetNamespace ...
func (d *Database) SetNamespace(ns string) {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.currentNamespace = ns
}

// WithPath ...
func WithPath(path string) Opt {
	return func(d *Database) {
		d.lock.Lock()
		defer d.lock.Unlock()

		d.path = path
	}
}

// WithNamespaces ...
func WithNamespaces(namespaces ...string) Opt {
	return func(d *Database) {
		d.lock.Lock()
		defer d.lock.Unlock()

		d.namespaces = append(d.namespaces, namespaces...)
	}
}

// WithBadgerOptions ...
func WithBadgerOptions(opts badger.Options) Opt {
	return func(d *Database) {
		d.lock.Lock()
		defer d.lock.Unlock()

		d.options = opts
	}
}

// WithContext ...
func WithContext(ctx context.Context) Opt {
	return func(d *Database) {
		d.lock.Lock()
		defer d.lock.Unlock()

		nCTX, cancel := context.WithCancel(ctx)
		d.ctx = nCTX
		d.cancel = cancel
	}
}

// WithNamespace ...
func WithNamespace(namespace string) Opt {
	return func(d *Database) {
		d.lock.Lock()
		defer d.lock.Unlock()

		d.currentNamespace = namespace
	}
}

// WithDiscardRatio ...
func WithDiscardRatio(ratio float64) Opt {
	return func(d *Database) {
		d.lock.Lock()
		defer d.lock.Unlock()

		d.discardRatio = ratio
	}
}
