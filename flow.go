// Package flow provides support for very basic FBP / pipelines. Each handler represents async
// stage consuming data from the input channel and publishing results to the output channel.
// Each handler runs in separate goroutines.
//
// User must implement Handler function and add it to the Flow. Each handler usually creates an output channel, reads from
// the input channel, processes data and sends results to the output channel. Processing sequence defined by order of those handlers.
//
// Any Handler can run in multiple concurrent goroutines (workers) by using the Parallel decorator.
//
// FanOut allows to pass multiple handlers in the broadcast mode, i.e., each handler gets every input record.
// Outputs from these handlers merged and combined into a single output channel.
//
// Processing error detected as return error value from user's handler func. Such error interrupts all other
// running handlers gracefully and won't keep any goroutine running/leaking.
//
// Each Flow object can be executed only once.
//
// Handler has to handle context cancellation as a termination signal.
package flow

import (
	"context"
	"log"
	"sync"

	"github.com/go-pkgz/flow/errgroup"
)

// Flow object with list of all runnable functions and common context
type Flow struct {
	group *errgroup.Group // all handlers runs in this errgroup
	ctx   context.Context // context used for cancellation

	lastCh chan interface{} // last channel in flow
	funcs  []func() error   // all runnable functions

	fanoutBuffer int       // buffer size for fanout
	activateOnce sync.Once // prevents multiple activations of flow
}

// Handler defines function type used as flow stages, implementations of handler provided by the user.
// Each handler returns the new out(put) channel and runnable fn function.
// fn will be executed in a separate goroutine. fn is thread-safe and may have mutable state. It will live
// all flow lifetime and usually implements read->process->write cycle. If fn returns != nil it indicates
// critical failure and will stop, with canceled context, all handlers in the flow.
type Handler func(ctx context.Context, in chan interface{}) (out chan interface{}, runFn func() error)

// New creates flow object with context and common errgroup. This errgroup used to schedule and cancel all handlers.
// options defines non-default parameters.
func New(options ...Option) *Flow {

	// default flow object parameters
	result := Flow{
		ctx:          context.Background(),
		fanoutBuffer: 1,
	}

	// apply options
	for _, opt := range options {
		opt(&result)
	}

	wg, ctx := errgroup.WithContext(result.ctx)
	result.ctx = context.WithValue(ctx, MetricsContextKey, NewMetrics())
	result.group = wg

	return &result
}

// Add one or more handlers. Each will be linked to the previous one and order of handlers defines sequence of stages in the flow.
// can be called multiple times.
func (f *Flow) Add(handlers ...Handler) *Flow {

	for _, handler := range handlers {
		ch, fn := handler(f.ctx, f.lastCh)
		if ch == nil {
			log.Fatalf("[ERROR] can't register flow handler with nil channel!")
		}
		f.lastCh = ch
		f.funcs = append(f.funcs, fn) // register runnable with flow executor
	}
	return f
}

// Parallel is a decorator, converts & adopts single handler to concurrently executed (parallel) handler.
// First it makes multiple handlers, registers all of them with common input channel as workers
// and then merges their output channels into single out channel (fan-in)
func (f *Flow) Parallel(concurrent int, handler Handler) Handler {

	if concurrent <= 1 { // not really parallel
		return handler
	}

	return func(ctx context.Context, ch chan interface{}) (chan interface{}, func() error) {
		var outChs []chan interface{}
		for n := 0; n < concurrent; n++ {
			ctxWithID := context.WithValue(ctx, CidContextKey, n) // put n as id to context for parallel handlers
			out, fn := handler(ctxWithID, ch)                     // all parallel handlers read from the same lastCh
			f.funcs = append(f.funcs, fn)                         // register runnable with flow executor
			outChs = append(outChs, out)
		}

		return f.merge(ctx, outChs) // returns chan and mergeFn. will be registered in usual way, as all handlers do
	}
}

// FanOut runs all handlers against common input channel and results go to common output channel.
// This will broadcast each record to multiple handlers and each may process it in different way.
func (f *Flow) FanOut(handler Handler, handlers ...Handler) Handler {

	// handlers params split as head handler and tail handlers. This is done just to prevent empty list of handlers
	// to be passed and ensure at least one handler.

	if len(handlers) == 0 {
		return handler
	}

	return func(ctx context.Context, ch chan interface{}) (chan interface{}, func() error) {

		handlers = append([]Handler{handler}, handlers...) // add head handler to head

		inChs := make([]chan interface{}, len(handlers))  // input channels for forked input from ch
		outChs := make([]chan interface{}, len(handlers)) // output channels for merging

		for i := 0; i < len(handlers); i++ {
			inChs[i] = make(chan interface{}, f.fanoutBuffer)     // buffered to allow async readers
			ctxWithID := context.WithValue(ctx, CidContextKey, i) // keep i as ID for handler in context
			out, fn := handlers[i](ctxWithID, inChs[i])           // handle forked input
			f.funcs = append(f.funcs, fn)                         // register runnable with flow executor
			outChs[i] = out                                       // collect all output channels
		}

		// broadcast every record from input channel ch to multiple, forked channels
		go func() {
			defer func() {
				for _, in := range inChs {
					close(in)
				}
			}()

			for e := range ch {
				// blocked, but buffered to allow async (optional) iteration
				for _, in := range inChs {
					if err := Send(ctx, in, e); err != nil {
						return
					}
				}
			}
		}()

		return f.merge(ctx, outChs) // returns merged chan and mergeFn. will be registered in usual way, as all handlers do
	}
}

// Go activates flow. Should be called exactly once after all handlers added, next calls ignored.
func (f *Flow) Go() *Flow {
	f.activateOnce.Do(func() {
		for _, fn := range f.funcs {
			if fn != nil { // in rare cases handler may rerun nil fn if nothing runnable (async) needed
				f.group.Go(fn)
			}
		}
	})
	return f
}

// Wait for completion, returns error if any happened in handlers.
func (f *Flow) Wait() error {
	return f.group.Wait()
}

// Channel returns last (final) channel in flow. Usually consumers don't need this channel, but can be used
// to return some final result(s)
func (f *Flow) Channel() chan interface{} {
	return f.lastCh
}

// Metrics returns all user-defined counters from the context.
func (f *Flow) Metrics() *Metrics {
	return f.ctx.Value(MetricsContextKey).(*Metrics)
}

// merge gets multiple channels and fan-in to a single output channel
func (f *Flow) merge(ctx context.Context, chs []chan interface{}) (chan interface{}, func() error) {

	mergeCh := make(chan interface{})
	mergeFn := func() error {
		defer close(mergeCh)

		gr, ctxGroup := errgroup.WithContext(ctx)
		for _, ch := range chs {
			ch := ch
			gr.Go(func() error {
				for e := range ch {
					if err := Send(ctxGroup, mergeCh, e); err != nil {
						return err
					}
				}
				return nil
			})
		}
		return gr.Wait()
	}
	return mergeCh, mergeFn
}

// CID returns concurrentID set by parallel wrapper
// The only use for cid is to alo some indication/logging.
func CID(ctx context.Context) int {
	cid, ok := ctx.Value(CidContextKey).(int)
	if !ok { // for non-parallel won't have any
		cid = 0
	}
	return cid
}

// Send entry to channel or returns error if context canceled.
// Shortcut for send-or-fail-on-cancel most handlers implement.
func Send(ctx context.Context, ch chan interface{}, e interface{}) error {
	select {
	case ch <- e:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Recv gets entry from the channel or returns error if context canceled.
// Shortcut for read-or-fail-on-cancel most handlers implement.
func Recv(ctx context.Context, ch chan interface{}) (interface{}, error) {
	select {
	case val := <-ch:
		return val, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
