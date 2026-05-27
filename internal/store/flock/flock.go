// Package flock provides advisory file locks.
package flock

import "sync"

type Flock struct {
	path string
	lock platformLock
	mu   sync.Mutex
}

func New(path string) *Flock {
	return &Flock{path: path}
}

func (f *Flock) Lock() error {
	f.mu.Lock()
	if err := f.lock.lock(f.path); err != nil {
		f.mu.Unlock()
		return err
	}
	return nil
}

func (f *Flock) Unlock() error {
	err := f.lock.unlock()
	f.mu.Unlock()
	return err
}
