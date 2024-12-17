package flow

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFlowWithAdd(t *testing.T) {

	f := New()

	f.Add(
		seedHandler,
		multiplierHandler(5, 0),
		multiplierHandler(3, 0),
		f.Parallel(10, multiplierHandler(2, 0)),
		collectorHandler,
	)
	log.Printf("funcs = %d", len(f.funcs))
	f.Go()

	e := f.Wait()
	assert.Nil(t, e)

	expected := ((100 - 1 + 1) * (1 + 100) / 2) * 5 * 3 * 2 // 151500
	sum := <-f.Channel()
	assert.Equal(t, expected, sum.(int))

	assert.Equal(t, 3*100, f.Metrics().Get("iters"))
	t.Log(f.Metrics())
}

func TestFlowFailed(t *testing.T) {
	f := New()

	f.Add(
		seedHandler,
		multiplierHandler(5, 0),
		multiplierHandler(3, 200),
		f.Parallel(10, multiplierHandler(2, 0)),
		collectorHandler,
	).Go()

	e := f.Wait()
	assert.EqualError(t, e, "oh my, failed")

	sum := <-f.Channel()
	assert.True(t, sum.(int) < 151500, "partial sum")
}

func TestFlowWithFanOut(t *testing.T) {

	f := New()

	f.Add(
		func(ctx context.Context, ch chan interface{}) (chan interface{}, func() error) {
			inp := make(chan interface{}, 100)
			for i := 1; i <= 100; i++ {
				inp <- 1
			}
			close(inp)
			return inp, nil
		},
		f.FanOut(multiplierHandler(5, 0), multiplierHandler(2, 0), multiplierHandler(3, 0)),
		collectorHandler,
	)
	log.Printf("funcs = %d", len(f.funcs))
	f.Go()

	e := f.Wait()
	assert.Nil(t, e)

	sum := <-f.Channel()
	assert.Equal(t, 500+200+300, sum.(int))

	assert.Equal(t, 100*3, f.Metrics().Get("iters"))
	t.Log(f.Metrics())
}

func TestFlowWithFanOutAndParallel(t *testing.T) {

	f := New(FanOutSize(10))

	f.Add(
		func(ctx context.Context, ch chan interface{}) (chan interface{}, func() error) {
			inp := make(chan interface{}, 100)
			for i := 1; i <= 100; i++ {
				inp <- 1
			}
			close(inp)
			return inp, nil
		},
		f.FanOut(
			f.Parallel(5, multiplierHandler(2, 0)),
			f.Parallel(2, multiplierHandler(3, 0)),
		),
		collectorHandler,
	)
	log.Printf("funcs = %d", len(f.funcs))
	f.Go()

	e := f.Wait()
	assert.Nil(t, e)

	sum := <-f.Channel()
	assert.Equal(t, 200+300, sum.(int))

	assert.Equal(t, 100*2, f.Metrics().Get("iters"))
	t.Log(f.Metrics())
}

func TestFlowWithInputAndSecondFlow(t *testing.T) {

	inp := make(chan interface{}, 100)
	for i := 1; i <= 100; i++ {
		inp <- 1
	}
	close(inp)

	f1 := New(Input(inp))
	f1.Add(multiplierHandler(2, 0))
	f1.Go()

	f2 := New(Input(f1.Channel()))
	f2.Add(multiplierHandler(3, 0), collectorHandler)
	f2.Go()

	e1 := f1.Wait()
	assert.Nil(t, e1)

	e2 := f2.Wait()
	assert.Nil(t, e2)

	sum := <-f2.Channel()
	assert.Equal(t, 100*2*3, sum.(int))

	assert.Equal(t, 100, f1.Metrics().Get("iters"))
	assert.Equal(t, 100, f2.Metrics().Get("iters"))
}

func TestFlowWithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	slowHandler := func(ctx context.Context, ch chan interface{}) (chan interface{}, func() error) {
		st := time.Now()
		resCh := make(chan interface{})
		resFn := func() error {
			defer close(resCh)
			processed := 0
			for inp := range ch {

				// slow operation
				select {
				case <-ctx.Done():
					log.Printf("error %s, [%02d] - %d", ctx.Err(), CID(ctx), processed)
					return ctx.Err()
				case <-time.After(time.Second):
				}

				resCh <- inp
				processed++

			}
			log.Println("slow handler completed", CID(ctx), time.Since(st), processed)
			return nil
		}
		return resCh, resFn
	}

	f := New(Context(ctx))
	f.Add(
		seedHandler,
		f.Parallel(40, slowHandler),
		collectorHandler,
	)

	st := time.Now()
	err := f.Go().Wait()
	t.Logf("%s", time.Since(st))
	assert.EqualError(t, err, "context deadline exceeded")
	assert.True(t, time.Since(st) < 3010*time.Millisecond)
}

func seedHandler(_ context.Context, _ chan interface{}) (chan interface{}, func() error) {
	inp := make(chan interface{}, 100)
	for i := 1; i <= 100; i++ {
		inp <- i
	}
	close(inp)
	return inp, nil
}

func multiplierHandler(mult, cancelOn int) Handler {

	fn := func(ctx context.Context, ch chan interface{}) (chan interface{}, func() error) {
		resCh := make(chan interface{})
		metrics := ctx.Value(MetricsContextKey).(*Metrics)

		resFn := func() error {
			log.Print("start multilayer x", mult)
			defer func() {
				log.Print("completed x", mult)
				close(resCh)
			}()

			for e := range ch {
				metrics.Add("iters", 1)
				val := e.(int)
				if cancelOn > 0 && val >= cancelOn {
					return errors.New("oh my, failed")
				}
				// send to resCh with flow.Send helper. Returns error on canceled ctx
				if err := Send(ctx, resCh, val*mult); err != nil {
					log.Print("term ", mult, err)
					return err
				}
			}

			return nil
		}
		return resCh, resFn
	}

	return fn
}

func collectorHandler(ctx context.Context, ch chan interface{}) (chOut chan interface{}, fnRun func() error) {
	resCh := make(chan interface{}, 1)

	calls := 0
	resFn := func() error {
		sum := 0
		log.Print("start collector")
		defer func() {
			resCh <- sum
			close(resCh)
			log.Printf("completed collector, %d (calls:%d)", sum, calls)
		}()

		for {
			select {
			case e, more := <-ch:
				if !more {
					return nil
				}
				val := e.(int)
				sum += val
				calls++

			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return resCh, resFn
}

// flow illustrates the use of a Flow for concurrent pipeline running each handler in separate goroutine.
func ExampleFlow_flow() {

	f := New() // create new empty Flow
	f.Add(     // add handlers. Note: handlers can be added directly in New

		// generate 100 initial values.
		func(ctx context.Context, in chan interface{}) (out chan interface{}, runFn func() error) {
			// example of non-async handler, Add executes it right away, prior to Go call
			out = make(chan interface{}, 100)
			for i := 1; i <= 100; i++ {
				out <- i
			}
			close(out)      // each handler has to close out channel
			return out, nil // no runnable function for non-async handler
		},

		// pick odd numbers only and multiply
		func(ctx context.Context, in chan interface{}) (out chan interface{}, runFn func() error) {
			out = make(chan interface{}) // each handler makes its out channel
			runFn = func() error {       // async handler returns runnable func
				defer close(out) // handler should close out channel
				for e := range in {
					val := e.(int)
					if val%2 == 0 {
						continue
					}
					f.Metrics().Inc("passed") // increment user-define metric "passed"

					// send result to the next stage with flow.Send helper. Also, checks for cancellation
					if err := Send(ctx, out, val*rand.Int()); err != nil { //nolint
						return err
					}
				}
				return nil
			}
			return out, runFn
		},

		// sum all numbers
		func(ctx context.Context, in chan interface{}) (out chan interface{}, runFn func() error) {
			out = make(chan interface{}, 1)
			runFn = func() error {
				defer close(out)
				sum := 0
				for {
					select {
					case e, more := <-in:
						if !more {
							out <- sum // send result
							return nil
						}
						val := e.(int)
						sum += val

					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}
			return out, runFn
		},
	)

	f.Go() // activate flow

	// wait for all handlers to complete
	if err := f.Wait(); err == nil {
		fmt.Printf("all done, result=%v, passed=%d", <-f.Channel(), f.Metrics().Get("passed"))
	}
}

// parallel illustrates the use of a Flow for concurrent pipeline running some handlers in parallel way.
func ExampleFlow_parallel() {

	f := New() // create new empty Flow

	// make flow with mixed singles and parallel handlers and activate
	f.Add(

		// generate 100 initial values in single handler
		func(ctx context.Context, in chan interface{}) (out chan interface{}, runFn func() error) {
			out = make(chan interface{}, 100) // example of non-async handler
			for i := 1; i <= 100; i++ {
				out <- i
			}
			close(out)      // each handler has to close out channel
			return out, nil // no runnable function for non-async handler
		},

		// multiple all numbers in 10 parallel handlers
		f.Parallel(10, func(ctx context.Context, in chan interface{}) (out chan interface{}, runFn func() error) {
			out = make(chan interface{}) // async handler makes its out channel
			runFn = func() error {
				defer close(out) // handler should close out channel
				for e := range in {
					val := e.(int)
					select {
					// send result to the next stage
					case out <- val * rand.Int(): //nolint
					case <-ctx.Done(): // check for cancellation
						return ctx.Err()
					}
				}
				return nil
			}
			return out, runFn
		}),

		// print all numbers
		func(ctx context.Context, in chan interface{}) (out chan interface{}, runFn func() error) {
			runFn = func() error {
				defer close(out)
				sum := 0
				for e := range in {
					val := e.(int)
					sum += val
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
					}
				}
				fmt.Printf("all done, result=%d", sum)
				return nil
			}
			return out, runFn
		},
	)

	// wait for all handlers to complete
	if err := f.Wait(); err == nil {
		fmt.Printf("all done, result=%v", <-f.Channel())
	}
}

// fanOut illustrates the use of a Flow for fan-out pipeline running same input via multiple handlers.
func ExampleFlow_fanOut() {

	f := New() // create new empty Flow

	f.Add( // add handlers. Note: handlers can be added directly in New

		// generate 100 ones.
		func(ctx context.Context, in chan interface{}) (out chan interface{}, runFn func() error) {
			out = make(chan interface{}, 100) // example of non-async handler
			for i := 1; i <= 100; i++ {
				out <- 1
			}
			close(out)      // each handler has to close out channel
			return out, nil // no runnable function for non-async handler
		},

		// fanout 100 ones and fork processing for odd and even numbers.
		// each input number passed to both handlers. output channels from both handlers merged together.
		f.FanOut(

			// first handler picks odd numbers only and multiply by 2
			func(ctx context.Context, in chan interface{}) (out chan interface{}, runFn func() error) {
				out = make(chan interface{}) // async handler makes its out channel
				runFn = func() error {
					defer close(out) // handler should close out channel
					for e := range in {
						val := e.(int)
						if val%2 != 0 {
							continue
						}
						f.Metrics().Inc("passed odd") // increment user-define metric "passed odd"
						select {
						case out <- val * 2: // send result to the next stage
						case <-ctx.Done(): // check for cancellation
							return ctx.Err()
						}
					}
					return nil
				}
				return out, runFn
			},

			// second handler picks even numbers only and multiply by 3
			func(ctx context.Context, in chan interface{}) (out chan interface{}, runFn func() error) {
				out = make(chan interface{}) // async handler makes its out channel
				runFn = func() error {
					defer close(out) // handler should close out channel
					for e := range in {
						val := e.(int)
						if val%2 == 0 {
							continue
						}
						f.Metrics().Inc("passed even") // increment user-define metric "passed even"
						select {
						case out <- val * 3: // send result to the next stage
						case <-ctx.Done(): // check for cancellation
							return ctx.Err()
						}
					}
					return nil
				}
				return out, runFn
			},
		),

		// sum all numbers
		func(ctx context.Context, in chan interface{}) (out chan interface{}, runFn func() error) {
			out = make(chan interface{}, 1)
			runFn = func() error {
				defer close(out)
				sum := 0
				// same loop as above, illustrates different way of reading from channel.
				for {
					select {
					case val, ok := <-in:
						if !ok {
							out <- sum // send result
							return nil
						}
						sum += val.(int)
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}
			return out, runFn
		},
	)

	f.Go() // activate flow

	// wait for all handlers to complete
	if err := f.Wait(); err == nil {
		fmt.Printf("all done, result=%v, odd=%d, even=%d",
			<-f.Channel(), f.Metrics().Get("passed odd"), f.Metrics().Get("passed even"))
	}
}
