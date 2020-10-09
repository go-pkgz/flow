// Package pool implements simplified, single-stage flow.
// By default it runs non-buffered channels, randomly distributed pool, i.e. incoming records send to one of workers randomly.
// User may define ChunkFn returning key portion of the record and in this case record will be send to workers based on this key
// and identical keys guaranteed to be send to the same worker. Batch option sets size of internal buffer to minimize channel sends.
// Batch collects incoming records per worker and send them in as a slice. Metrics can be retrieved by user
// with Metrics and updated.
//
// Workers pool should not be reused and can be activated only once.
// Thread safe, no additional locking needed.
package pool

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"math/rand"
	"time"

	"github.com/go-pkgz/flow"
	"golang.org/x/sync/errgroup"
)

// Workers is a simple case of flow with a single stage only.
type Workers struct {
	poolSize  int // number of workers (goroutines)
	batchSize int // size of batch send to workers

	chunkFn         func(interface{}) string
	resChanSize     int        // size of responses channel
	workerChanSize  int        // size of worker channels
	workerFn        WorkerFn   // worker function
	completeFn      CompleteFn // completion callback function
	continueOnError bool       // don't terminate on first error

	store []WorkerStore // workers store, per worker ID

	buf       [][]interface{}
	workersCh []chan []interface{}
	ctx       context.Context
	eg        *errgroup.Group
}

// response wraps data and error
type response struct {
	value interface{} // the actual data
	err   error       // optional error
}

// WorkerStore defines interface for per-worker storage
type WorkerStore interface {
	Set(key string, val interface{})
	Get(key string) (interface{}, bool)
	GetInt(key string) int
	GetFloat(key string) float64
	GetString(key string) string
	GetBool(key string) bool
	Keys() []string
	Delete(key string)
}

type contextKey string

const widContextKey contextKey = "worker-id"

// WorkerFn processes input record inpRec and optionally sends results to sender func
type WorkerFn func(ctx context.Context, inpRec interface{}, sender SenderFn, store WorkerStore) error

// SenderFn func called by worker code to publish results
type SenderFn func(val interface{}) error

// CompleteFn processes input record inpRec and optionally sends response to respCh
type CompleteFn func(ctx context.Context, store WorkerStore) error

// New creates worker pool, can be activated once
func New(poolSize int, workerFn WorkerFn, options ...Option) *Workers {

	if poolSize < 1 {
		poolSize = 1
	}

	res := Workers{
		poolSize:       poolSize,
		workersCh:      make([]chan []interface{}, poolSize),
		buf:            make([][]interface{}, poolSize),
		store:          make([]WorkerStore, poolSize),
		workerFn:       workerFn,
		completeFn:     nil,
		chunkFn:        nil,
		batchSize:      1,
		resChanSize:    0,
		workerChanSize: 0,
	}

	// apply options
	for _, opt := range options {
		opt(&res)
	}

	// initialize workers channels and batch buffers
	for id := 0; id < poolSize; id++ {
		res.workersCh[id] = make(chan []interface{}, res.workerChanSize)
		if res.batchSize > 1 {
			res.buf[id] = make([]interface{}, 0, poolSize)
		}
		res.store[id] = NewLocalStore()
	}

	rand.Seed(time.Now().UnixNano())
	return &res
}

// Submit record to pool, can be blocked
func (p *Workers) Submit(v interface{}) {

	// randomize distribution by default
	id := rand.Intn(p.poolSize) //nolint gosec
	if p.chunkFn != nil {
		// chunked distribution
		id = int(crc32.Checksum([]byte(p.chunkFn(v)), crc32.MakeTable(crc32.IEEE))) % p.poolSize
	}

	if p.batchSize <= 1 {
		// skip all buffering if batch size is 1 or less
		p.workersCh[id] <- append([]interface{}{}, v)
		return
	}

	p.buf[id] = append(p.buf[id], v) // add to batch buffer
	if len(p.buf[id]) >= p.batchSize {
		// commit copy to workers
		cp := make([]interface{}, len(p.buf[id]))
		copy(cp, p.buf[id])
		p.workersCh[id] <- cp
		p.buf[id] = p.buf[id][:0] // reset size, keep capacity
	}
}

// Go activates worker pool, closes result chan on completion
func (p *Workers) Go(ctx context.Context) (Cursor, error) {
	if p.ctx != nil {
		return Cursor{}, errors.New("workers poll already activated")
	}

	respCh := make(chan response, p.resChanSize)
	p.ctx = context.WithValue(ctx, flow.MetricsContextKey, flow.NewMetrics())
	var egCtx context.Context
	p.eg, egCtx = errgroup.WithContext(ctx)
	worker := func(id int, inCh chan []interface{}) func() error {
		return func() error {
			wCtx := context.WithValue(p.ctx, widContextKey, id)
			for {
				select {
				case vv, ok := <-inCh:
					if !ok { // input channel closed
						e := p.flush(wCtx, id, respCh)
						if !p.continueOnError {
							return e
						}
						return nil
					}

					// read from the input slice
					for _, v := range vv {
						if err := p.workerFn(wCtx, v, p.sendResponseFn(wCtx, respCh), p.store[id]); err != nil {
							e := fmt.Errorf("worker %d failed: %w", id, err)
							if !p.continueOnError {
								return e
							}
							respCh <- response{err: e}
						}
					}

				case <-ctx.Done(): // parent context, passed by caller
					respCh <- response{err: ctx.Err()}
					return ctx.Err()
				case <-egCtx.Done(): // worker context, set by errgroup
					if !p.continueOnError {
						return egCtx.Err()
					}
					return nil
				}
			}
		}
	}

	// start pool goroutines
	for i := 0; i < p.poolSize; i++ {
		p.eg.Go(worker(i, p.workersCh[i]))
	}

	go func() {
		// wait for completion and close the channel
		if err := p.eg.Wait(); err != nil {
			respCh <- response{err: err}
		}
		close(respCh)
	}()

	return Cursor{ch: respCh}, nil
}

// Metrics returns all user-defined counters from context.
func (p *Workers) Metrics() *flow.Metrics {
	return Metrics(p.ctx)
}

// flush all records left in buffer to workers, called once for each worker
func (p *Workers) flush(ctx context.Context, id int, ch chan response) (err error) {
	for _, v := range p.buf[id] {
		if e := p.workerFn(ctx, v, p.sendResponseFn(ctx, ch), p.store[id]); e != nil {
			err = fmt.Errorf("worker %d failed in flush: %w", id, e)
			if !p.continueOnError {
				return err
			}
			ch <- response{err: err}
		}
	}
	p.buf[id] = p.buf[id][:0] // reset size to 0

	// call completeFn for given worker id
	if p.completeFn != nil {
		if e := p.completeFn(ctx, p.store[id]); e != nil {
			err = fmt.Errorf("complete func for %d failed: %w", id, e)
		}
	}

	return err
}

// Close pool. Has to be called by consumer as the indication of "all records submitted".
// after this call poll can't be reused.
func (p *Workers) Close() {
	for _, ch := range p.workersCh {
		close(ch)
	}
}

// Wait till workers completed and result channel closed
func (p *Workers) Wait(ctx context.Context) (err error) {
	doneCh := make(chan error)
	go func() {
		doneCh <- p.eg.Wait()
	}()

	for {
		select {
		case err := <-doneCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// sendResponseFn makes sender func used by worker
func (p *Workers) sendResponseFn(ctx context.Context, respCh chan response) func(val interface{}) error {
	return func(val interface{}) error {
		select {
		case respCh <- response{value: val}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Metrics from context
func Metrics(ctx context.Context) *flow.Metrics {
	res, ok := ctx.Value(flow.MetricsContextKey).(*flow.Metrics)
	if !ok {
		return flow.NewMetrics()
	}
	return res
}

// WorkerID returns worker ID from the context
func WorkerID(ctx context.Context) int {
	cid, ok := ctx.Value(widContextKey).(int)
	if !ok { // for non-parallel won't have any
		cid = 0
	}
	return cid
}
