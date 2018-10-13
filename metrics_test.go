package flow

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetrics(t *testing.T) {
	m := NewMetrics()

	m.Add("k1", 100)
	m.Inc("k1")
	m.Inc("k2")
	m.Set("k3", 12345)

	t.Log(m)

	assert.Equal(t, 101, m.Get("k1"))
	assert.Equal(t, 1, m.Get("k2"))
	assert.Equal(t, 12345, m.Get("k3"))
	assert.Equal(t, 0, m.Get("kn"))

	assert.True(t, strings.HasSuffix(m.String(), "[k1:101, k2:1, k3:12345]"))
}
