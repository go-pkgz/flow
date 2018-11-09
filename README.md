# Flow - FBP / pipelines [![Build Status](https://travis-ci.org/go-pkgz/flow.svg?branch=master)](https://travis-ci.org/go-pkgz/flow) [![Go Report Card](https://goreportcard.com/badge/github.com/go-pkgz/flow)](https://goreportcard.com/report/github.com/go-pkgz/flow) [![Coverage Status](https://coveralls.io/repos/github/go-pkgz/flow/badge.svg?branch=master)](https://coveralls.io/github/go-pkgz/flow?branch=master)

Package `flow` provides support for very basic FBP / pipelines. It helps to structure multistage processing as 
a set of independent handlers communicating via channels. The typical use case is for ETL (extract, transform, load)
type of processing.

Package `flow` doesnt't introduce any high-level abstraction and keeps everything in the hand of the user. 

## Details

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

## Example of the handler

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

## Usage

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