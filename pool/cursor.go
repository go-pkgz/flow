package pool

import "context"

// Cursor provides synchronous access to async data from response channel
type Cursor struct {
	ch  chan response
	err error
}

// Next returns next result from the cursor, ok = false on completion.
// Any error saved internally and can be returned by Err call
func (c *Cursor) Next(ctx context.Context, v *interface{}) bool {
	for {
		select {
		case resp, ok := <-c.ch:
			if !ok {
				return false
			}
			if resp.err != nil {
				c.err = resp.err
				continue
			}
			*v = resp.value
			return ok
		case <-ctx.Done():
			c.err = ctx.Err()
			return false
		}
	}
}

// All gets all data from the cursor
func (c *Cursor) All(ctx context.Context) (res []interface{}, err error) {
	var v interface{}
	for c.Next(ctx, &v) {
		res = append(res, v)
	}
	return res, c.err
}

// Err returns error collected by Next
func (c *Cursor) Err() error { return c.err }
