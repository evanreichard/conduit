package maps

import (
	"iter"
	"sync"
)

type Map[K comparable, V any] struct {
	items map[K]V
	mu    sync.RWMutex
}

func New[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{items: make(map[K]V)}
}

func (m *Map[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.items[key]
	return v, ok
}

func (m *Map[K, V]) Set(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = value
}

func (m *Map[K, V]) Delete(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key)
}

func (m *Map[K, V]) HasKey(key K) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.items[key]
	return ok
}

func (m *Map[K, V]) Entries() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		m.mu.RLock()
		defer m.mu.RUnlock()
		for k, v := range m.items {
			if !yield(k, v) {
				return
			}
		}
	}
}
