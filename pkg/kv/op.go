package kv

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dgraph-io/badger/v4"
)

// enforce implementation of the interface
var _ Store = (*Database)(nil)

// Get implements the Store interface for Database. It retrieves a key from the database with
// the given OperationOpt options to configure the current operation. The operation itself is
// handled asynchronously, although the method itself is also thread-safe.
func (d *Database) Get(key string) (value []byte, err error) {
	// ensure we're getting clean keys
	if err := d.ensureNonNamespaced(key); err != nil {
		return nil, err
	}

	return d.get([]byte(key))
}

// get is the actual implementation of the retrieval of a value from the BadgerDB
// database. The key itself is namespaced beforehand to allow for multiple data
// models to be persisted simultaneously.
func (d *Database) get(key []byte) (value []byte, err error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	var bytes []byte
	k := d.namespace(d.currentNamespace, string(key))
	err = d.kv.View(func(txn *badger.Txn) error {
		item, err := txn.Get(k)
		if err != nil {
			return err
		}

		value, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}

		bytes = append(bytes, value...)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return bytes, nil
}

// Set implements the Store interface for Database. It sets a key within the database to
// a certain value with the given OperationOpt options to configure the current operation.
// The operation itself is handled asynchronously, although the method itself is also thread-safe.
func (d *Database) Set(key string, value []byte) error {
	if err := d.ensureNonNamespaced(key); err != nil {
		return err
	}

	return d.set([]byte(key), value)
}

// get is the actual implementation of setting a value within the BadgerDB
// database. The key itself is namespaced beforehand to allow for multiple data
// models to be persisted simultaneously.
func (d *Database) set(key, value []byte) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	k := d.namespace(d.currentNamespace, string(key))
	var err = d.kv.Update(func(txn *badger.Txn) error {
		err := txn.Set(k, value)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// Has checks if the database contains a key by trying to read the value at the given key. A
// missing key is reported as (false, nil), not an error - only a genuine underlying failure is
// propagated as a non-nil error.
func (d *Database) Has(key string) (bool, error) {
	err := d.ensureNonNamespaced(key)
	if err != nil {
		return false, err
	}
	k := []byte(key)
	value, err := d.get(k)
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return false, nil
		}

		return false, err
	}

	return len(value) > 0, nil
}

// Namespaces returns the current namespaces the database has been initialized with.
// The function cannot error since by default only the (single) "default" namespace
// will be returned if the DB is otherwise largely unconfigured.
func (d *Database) Namespaces() []string {
	d.lock.Lock()
	defer d.lock.Unlock()

	return d.namespaces
}

// Delete deletes a key from the database
func (d *Database) Delete(key string) error {
	err := d.delete([]byte(key))
	if err != nil {
		return err
	}

	return nil
}

func (d *Database) delete(key []byte) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	k := d.namespace(d.currentNamespace, string(key))
	var err = d.kv.Update(func(txn *badger.Txn) error {
		err := txn.Delete(k)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// Close closes an active connection to a database. This method must be called
// during program shutdown or else you risk corrupting the data. Additionally,
// we run garbage collection across the entire database before finally shutting
// down.
func (d *Database) Close() error {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.gc()
	return d.kv.Close()
}

// Path returns the filesystem path to the database
func (d *Database) Path() string {
	d.lock.Lock()
	defer d.lock.Unlock()

	return d.path
}

// Config returns the badger.Options the database was initialized with
func (d *Database) Config() badger.Options {
	d.lock.Lock()
	defer d.lock.Unlock()

	return d.options
}

// namespace namespaces a given key by prefixing the value with a namespace like "namespace/value".
// If an empty string is passed as the namespace, the DefaultNamespace "default" is used instead.
func (d *Database) namespace(namespace, key string) []byte {
	if namespace == "" {
		namespace = DefaultNamespace
	}

	prefix := fmt.Sprintf("%s/", namespace)
	return []byte(prefix + key)
}

// ensure non-namespaced ensures that a given key value contains no slashes, thereby preventing
// callers from passing an already-namespaced key (namespace() applies its own "namespace/" prefix).
func (d *Database) ensureNonNamespaced(key string) error {
	if strings.Contains(key, "/") {
		return fmt.Errorf("cannot set value for a namespaced key. please exclude namespaces from the key")
	}

	return nil
}

// gc runs a single garbage-collection pass over the BadgerDB value log to reclaim filesystem
// space, repeating until there's nothing left to reclaim. It's called synchronously, right before
// closing the database connection.
func (d *Database) gc() {
	for {
		if err := d.kv.RunValueLogGC(0.7); err != nil {
			return
		}
	}
}
