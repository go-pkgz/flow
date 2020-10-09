package pool

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCursor_Next(t *testing.T) {
	c := Cursor{ch: make(chan response, 3)}

	c.ch <- response{value: "12345"}
	c.ch <- response{value: "abc"}
	c.ch <- response{value: "xyz 0987"}
	close(c.ch)

	var v interface{}
	next := c.Next(context.Background(), &v)
	assert.True(t, next)
	assert.Equal(t, "12345", v.(string))

	next = c.Next(context.Background(), &v)
	assert.True(t, next)
	assert.Equal(t, "abc", v.(string))

	next = c.Next(context.Background(), &v)
	assert.True(t, next)
	assert.Equal(t, "xyz 0987", v.(string))

	next = c.Next(context.Background(), &v)
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
