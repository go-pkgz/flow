package pool

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalStore(t *testing.T) {
	store := NewLocalStore()
	store.Set("key1", 123)

	v, ok := store.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, 123, v.(int))
	assert.Equal(t, 123, store.GetInt("key1"))
	assert.Equal(t, false, store.GetBool("key1"))
	assert.Equal(t, 0.0, store.GetFloat("key1"))
	assert.Equal(t, "", store.GetString("key1"))

	v, ok = store.Get("key2")
	assert.False(t, ok)
	assert.Equal(t, 0, store.GetInt("key2"))

	store.Set("key2", 123.34)
	assert.InDelta(t, 123.34, store.GetFloat("key2"), 0.001)
	assert.Equal(t, 0, store.GetInt("key2"))

	assert.Equal(t, []string{"key1", "key2"}, store.Keys())
	store.Delete("key1")
	assert.Equal(t, []string{"key2"}, store.Keys())
	assert.Equal(t, 0, store.GetInt("key1"))
}
