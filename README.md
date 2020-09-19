# Flow - FBP / pipelines / workers pool
 
[![Build Status](https://github.com/go-pkgz/flow/workflows/build/badge.svg)](https://github.com/go-pkgz/flow/actions) [![Go Report Card](https://goreportcard.com/badge/github.com/go-pkgz/flow)](https://goreportcard.com/report/github.com/go-pkgz/flow) [![Coverage Status](https://coveralls.io/repos/github/go-pkgz/flow/badge.svg?branch=master)](https://coveralls.io/github/go-pkgz/flow?branch=master)

Package `flow` provides support for very basic FBP / pipelines. It helps to structure multistage processing as 
a set of independent handlers communicating via channels. The typical use case is for ETL (extract, transform, load)
type of processing. Package `flow` doesn't introduce any high-level abstraction and keeps everything in the hand of the user. 


Package `pool` provides a simplified version of `flow` suitable for cases with a single-handler flows. 

## Details about `flow` package

- Each handler represents an async stage. It consumes data from an input channel and publishes results to an output channel. 
- Each handler runs in a separate goroutine. 
- User must implement Handler functions and add it to the Flow. 
- Each handler usually creates an output channel, reads from
the input channel, processes data, sends results to the output channel and closes the output channel.
- Processing sequence determined by the order of those handlers.
- Any `Handler` can run in multiple concurrent goroutines (workers) by using the `Parallel` decorator. 
- `FanOut` allows to pass multiple handlers in broadcast mode, i.e., each handler gets every input record. Outputs
from these handlers merged into single output channel.
- Processing error detected as return error value from user's handler func. Such error interrupts all other
running handlers gracefully and won't keep any goroutine running/leaking. 
- Each `Flow` object can be executed only once.
- `Handler` should handle context cancellation as a termination signal.

## Install and update

`go get -u github.com/go-pkgz/flow`

## Example of the flow's handler

```go
// ReaderHandler creates flow.Handler, reading strings from any io.Reader
func ReaderHandler(reader io.Reader) Handler {
	return func(ctx context.Context, ch chan interface{}) (chan interface{}, func() error) {
		metrics := flow.GetMetrics(ctx) // metrics collects how many records read with "read" key.

		readerCh := make(chan interface{}, 1000)
		readerFn := func() error {
			defer close(readerCh)

			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {

				select {
				case readerCh <- scanner.Text():
					metrics.Inc("read")
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return errors.Wrap(scanner.Err(), "scanner failed")
		}
		return readerCh, readerFn
	}
}
```

## Usage of the flow package

_for complete example see [example](https://github.com/go-pkgz/flow/tree/master/_example)_

```go
// flow illustrates the use of a Flow for concurrent pipeline running each handler in separate goroutine.
func ExampleFlow_flow() {

	f := New() // create new empty Flow
	f.Add(     // add handlers. Note: handlers can be added directly in New

		// first handler, generate 100 initial values.
		func(ctx context.Context, in chan interface{}) (out chan interface{}, runFn func() error) {
			out = make(chan interface{}, 100) // example of non-async handler
			for i := 1; i <= 100; i++ {
				out <- i
			}
			close(out)      // each handler has to close out channel
			return out, nil // no runnable function for non-async handler
		},

		// second handler - picks odd numbers only and multiply
		func(ctx context.Context, in chan interface{}) (out chan interface{}, runFn func() error) {
			out = make(chan interface{}) // async handler makes its out channel
			runFn = func() error {
				defer close(out) // handler should close out channel
				for e := range in {
					val := e.(int)
					if val%2 == 0 {
						continue
					}
					f.Metrics().Inc("passed") // increment user-define metric "passed"

					// send result to the next stage with flow.Send helper. Also checks for cancellation
					if err := Send(ctx, out, val*rand.Int()); err != nil {
						return err
					}
				}
				return nil
			}
			return out, runFn
		},

		// final handler - sum all numbers
		func(ctx context.Context, in chan interface{}) (out chan interface{}, runFn func() error) {
			out = make(chan interface{}, 1)
			runFn = func() error {
				defer close(out)
				sum := 0
				for {
					select {
					case e, more := <-in:
						if !more {
							out <- sum //send result
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
```

```go
// illustrates the use of a Flow for concurrent pipeline running some handlers in parallel way.
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
					case out <- val * rand.Int(): // send result to the next stage
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
```

## Details about `pool` package

- In addition to the default "run a func in multiple goroutines" mode, it also provides an optional support of chunked workers. It means - each key, detected by user-provide func guaranteed to be processed by the same worker. Such mode needed for stateful flows where each set of input records has to be processed sequentially and some state should be kept.
- another thing `pool` provides is a batch size. This one is a simple performance optimization keeping input request into a buffer and send them to worker channel in batches (slices) instead of per-submit call

Options:

- `ChunkFn` - the function returns string identifying the chunk
- `Batch` - sets batch size (default 1)
- `ChanResSize` sets the size of output buffered channel (default 1)
- `ChanWorkerSize` sets the size of workers buffered channel (default 1)
- `ContinueOnError` allows workers continuation after error occurred
- `OnCompletion` sets callback for each worker called on successful completion

### worker function

Worker function passed by user and will run in multiple workers (goroutines). 
This is the function: `type workerFn func(ctx context.Context, inp interface{}, resCh interface{}, store WorkerStore} error`

It takes `inp` parameter, does the job and optionally send result(s) to `resCh`. Error will terminate all workers.
Note: `workerFn` can be stateful, collect anything it needs and sends 0 or more results. Results wrapped in `Response` struct
allowing to communicate error code back to consumer.  `workerFn` doesn't need to send errors, enough just return non-nil error.
 
### worker store

Each worker gets `WorkerStore` and can be used as thread-safe per-worker storage for any intermediate results.

```go
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
```

_alternatively state can be kept outside of workers as a slice of values and accessed by worker ID._

### usage

```go
    p := pool.New(8, func(ctx context.Context, v interface{}, resCh interface{}, ws pool.WorkerStore} error {
        // worker function gets input v processes it and response(s) channel to send results

        input, ok := v.(string) // in this case it gets string as input
        if !ok {
            return errors.New("incorrect input type")
        }   
        // do something with input
        // ...
       
        v := ws.GetInt("something")  // access thread-local var
                
	    resCh <- pool.Response{Data: "foo"}
	    resCh <- pool.Response{Data: "bar"}
        pool.Metrics(ctx).Inc("counter")
        ws.Set("something", 1234) // keep thread-local things
       return "something", true, nil
    })
    
    ch := p.Go(context.TODO()) // start all workers in 8 goroutines
    
    // submit values (consumer side)
    go func() {
        p.Submit("something")
        p.Submit("something else")
        p.Close() // indicates completion of all inputs
    }()   

    for rec := range ch {
        if rec.Errors != nil { // error happened
            return err
        } 
        log.Print(rec.Data)  // print value      
    }

    // alternatively ReadAll helper can be used to get everything from response channel
    res, err := pool.ReadAll(ch)

    // metrics the same as for flow
    metrics := pool.Metrics()
    log.Print(metrics.Get("counter"))
```