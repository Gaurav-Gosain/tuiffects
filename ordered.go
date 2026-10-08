package tuiffects

import "strconv"

// orderedMap keeps insertion order alongside key lookup. Scene and path ids are
// auto-allocated by probing upward from the current count, and several effects
// walk the collection in the order they built it, so a plain Go map would
// change behaviour rather than just layout.
//
// Lookup is a linear scan while the map is small, deliberately. There is one of
// these per character for its scenes and another for its paths, and most hold
// a handful of entries: comparing three short strings beats hashing one, and
// it saves two map allocations per character. Over a full screen that is tens
// of thousands of maps that never existed.
//
// Some effects do put hundreds of entries in one. rings gives each character a
// path per ring cell, and a scan of those ids was 58% of its frame CPU. Past
// orderedMapIndexAt entries the map builds a hash index and looks up there.
type orderedMap[T any] struct {
	keys   []string
	values []*T
	// index maps a key to its position. It is nil until the map grows past
	// orderedMapIndexAt entries.
	index map[string]int
}

// orderedMapIndexAt is the size above which an orderedMap keeps a hash index.
const orderedMapIndexAt = 8

func newOrderedMap[T any]() orderedMap[T] { return orderedMap[T]{} }

func (m *orderedMap[T]) Len() int { return len(m.keys) }

func (m *orderedMap[T]) indexOf(key string) int {
	if m.index != nil {
		if i, ok := m.index[key]; ok {
			return i
		}
		return -1
	}
	for i, k := range m.keys {
		if k == key {
			return i
		}
	}
	return -1
}

func (m *orderedMap[T]) Has(key string) bool { return m.indexOf(key) >= 0 }

func (m *orderedMap[T]) Get(key string) *T {
	if i := m.indexOf(key); i >= 0 {
		return m.values[i]
	}
	return nil
}

// Set inserts or replaces. A replaced key keeps its original position, which is
// what a Python dict does on reassignment.
func (m *orderedMap[T]) Set(key string, value *T) {
	if i := m.indexOf(key); i >= 0 {
		m.values[i] = value
		return
	}
	m.keys = append(m.keys, key)
	m.values = append(m.values, value)
	switch {
	case m.index != nil:
		m.index[key] = len(m.keys) - 1
	case len(m.keys) > orderedMapIndexAt:
		m.index = make(map[string]int, 2*len(m.keys))
		for i, k := range m.keys {
			m.index[k] = i
		}
	}
}

func (m *orderedMap[T]) Keys() []string { return m.keys }

// nextAutoID reproduces upstream's id allocation: start at the current length
// and count up until an unused id turns up.
func (m *orderedMap[T]) nextAutoID() string {
	candidate := len(m.keys)
	for {
		id := strconv.Itoa(candidate)
		if m.indexOf(id) < 0 {
			return id
		}
		candidate++
	}
}

// Clear empties the map, keeping the backing arrays for reuse.
func (m *orderedMap[T]) Clear() {
	m.keys = m.keys[:0]
	m.values = m.values[:0]
	m.index = nil
}
