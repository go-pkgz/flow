package flow

import "context"

// Option func type
type Option func(f *Flow)

// Context functional option defines initial context.
// Can be used to add some values to context or set timeouts/deadlines.
func Context(ctx context.Context) Option {
	return func(f *Flow) {
		f.ctx = ctx
	}
}

// FanOutSize functional option defines size of fanout buffers.
func FanOutSize(size int) Option {
	return func(f *Flow) {
		if size < 1 {
			size = 1
		}
		f.fanoutBuffer = size
	}
}

// Input functional option defines input channels for first handler in chain.
// Can be used to connect multiple flows together or seed flow from the outside, with some external data.
func Input(ch chan interface{}) Option {
	return func(f *Flow) {
		f.lastCh = ch
	}
}
