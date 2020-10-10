package pool

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCursor_Next(t *testing.T) {

	type structExample struct {
		k1 int
		k2 string
		k3 []bool
	}

	c := Cursor{ch: make(chan response, 4)}

	c.ch <- response{value: "12345"}
	c.ch <- response{value: "abc"}
	c.ch <- response{value: 1234.567}
	c.ch <- response{value: structExample{k1: 12, k2: "abcd", k3: []bool{true, false}}}
	close(c.ch)

	var s string
	next := c.Next(context.Background(), &s)
	assert.True(t, next)
	assert.Equal(t, "12345", s)

	next = c.Next(context.Background(), &s)
	assert.True(t, next)
	assert.Equal(t, "abc", s)

	var f float64
	next = c.Next(context.Background(), &f)
	assert.True(t, next)
	assert.Equal(t, 1234.567, f)

	var ss structExample
	next = c.Next(context.Background(), &ss)
	assert.True(t, next)
	assert.Equal(t, structExample{k1: 12, k2: "abcd", k3: []bool{true, false}}, ss)

	next = c.Next(context.Background(), nil)
	assert.False(t, next)
}

func TestCursor_All(t *testing.T) {
	c := Cursor{ch: make(chan response, 3)}

	c.ch <- response{value: "12345"}
	c.ch <- response{value: "abc"}
	c.ch <- response{value: "xyz 0987"}
	close(c.ch)

	res, err := c.All(context.Background())
	require.NoError(t, err)
	assert.Len(t, res, 3)

	res, err = c.All(context.Background())
	require.NoError(t, err)
	assert.Len(t, res, 0)
}

func TestCursor_AllWithError(t *testing.T) {
	c := Cursor{ch: make(chan response, 3)}

	c.ch <- response{value: "12345"}
	c.ch <- response{value: "abc", err: errors.New("failed")}
	c.ch <- response{value: "xyz 0987"}
	close(c.ch)

	res, err := c.All(context.Background())
	require.EqualError(t, err, "failed")
	assert.Len(t, res, 2)

	res, err = c.All(context.Background())
	require.EqualError(t, err, "failed")
	assert.Len(t, res, 0)
}
