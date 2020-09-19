package pool

import (
	"sort"
)

type localStore struct {
	data map[string]interface{}
}

// NewLocalStore makes map-based worker store
func NewLocalStore() WorkerStore {
	return &localStore{data: map[string]interface{}{}}
}

// Set value for a given key
func (l *localStore) Set(key string, val interface{}) {
	l.data[key] = val
}

// Get value for a given key
func (l *localStore) Get(key string) (interface{}, bool) {
	val, ok := l.data[key]
	return val, ok
}

// GetInt value for a given key. If not found returns 0
func (l *localStore) GetInt(key string) int {
	val, ok := l.Get(key)
	if !ok {
		return 0
	}
	res, ok := val.(int)
	if !ok {
		return 0
	}
	return res
}

// GetFloat value for a given key. If not found returns 0
func (l *localStore) GetFloat(key string) float64 {
	val, ok := l.Get(key)
	if !ok {
		return 0
	}
	res, ok := val.(float64)
	if !ok {
		return 0
	}
	return res
}

// GetString value for a given key. If not found returns empty string
func (l *localStore) GetString(key string) string {
	val, ok := l.Get(key)
	if !ok {
		return ""
	}
	res, ok := val.(string)
	if !ok {
		return ""
	}
	return res
}

// GetBool value for a given key. If not found returns false
func (l *localStore) GetBool(key string) bool {
	val, ok := l.Get(key)
	if !ok {
		return false
	}
	res, ok := val.(bool)
	if !ok {
		return false
	}
	return res
}

// Keys returns list of all keys
func (l *localStore) Keys() []string {
	res := make([]string, 0, len(l.data))
	for k := range l.data {
		res = append(res, k)
	}
	sort.Strings(res)
	return res
}

// Delete key from the store
func (l *localStore) Delete(key string) {
	delete(l.data, key)
}
