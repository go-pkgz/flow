package main

import (
	"bufio"
	"context"
	"log"
	"os"
	"strings"

	"github.com/go-pkgz/flow"
	"github.com/pkg/errors"
)

// this toy example demonstrates a typical use of flow library.
// The goal is to read input files from data directory and process they content concurrently. The processing
// includes filtering short words and calculating the total size and count of the rest. It also calculates
// how many times word "data" appeared.

func main() {
	log.Printf("flow example started")

	// seed channel with the list of input files. Usually seeding implemented as a separate handler but for our toy example
	// filling a buffered channel will do it.
	seedCh := make(chan interface{}, 4)
	seedCh <- "data/input-1.txt"
	seedCh <- "data/input-2.txt"
	seedCh <- "data/input-3.txt"
	seedCh <- "data/input-4.txt"
	close(seedCh)

	f := flow.New(flow.Input(seedCh)) // make flow with input channel
	f.Add(
		f.Parallel(2, lineSplitHandler), // adds 2 parallel split handles
		f.Parallel(10, wordsHandler(3)), // adds 10 word handler
		sumHandler,                      // final handler, reducing results
	)
	f.Go()          // activate flow
	err := f.Wait() // wait for completion

	log.Printf("flow example finished with err=%v", err)
}

// lineSplitHandler gets file names and sends lines of text
func lineSplitHandler(ctx context.Context, ch chan interface{}) (chan interface{}, func() error) {
	log.Print("make line split handler")
	metrics := flow.GetMetrics(ctx)
	lineCh := make(chan interface{}, 100)
	lineFn := func() error {
		log.Printf("start line split handler %d", flow.CID(ctx))
		defer close(lineCh)
		for val := range ch {
			fname := val.(string)
			fh, err := os.Open(fname)
			if err != nil {
				return errors.Wrapf(err, "failed to open %s", fname)
			}

			lines := 0
			scanner := bufio.NewScanner(fh)
			for scanner.Scan() {
				if err = flow.Send(ctx, lineCh, scanner.Text()); err != nil {
					return ctx.Err()
				}
				lines++
			}

			metrics.Add("lines", lines)

			if e := fh.Close(); e != nil {
				log.Printf("failed to close %s, %v", fname, err)
			}

			log.Printf("file reader completed for %s, read %d lines (total %d)", fname, lines, metrics.Get("lines"))
			if scanner.Err() != nil {
				return errors.Wrapf(scanner.Err(), "scanner failed for %s", fname)
			}
		}
		log.Printf("line split handler completed, id=%d lines=%d", flow.CID(ctx), metrics.Get("lines"))
		return nil
	}
	return lineCh, lineFn
}

type wordsInfo struct {
	size    int
	special bool
}

// wordsHandler reads lines of text from the input and sends wordsInfo
func wordsHandler(minSize int) flow.Handler {
	log.Printf("make words handler with minsize=%d", minSize)
	return func(ctx context.Context, ch chan interface{}) (chan interface{}, func() error) {
		log.Printf("start words handler %d with minsize=%d", flow.CID(ctx), minSize)
		metrics := flow.GetMetrics(ctx)
		wordsCh := make(chan interface{}, 1000)
		wordsFn := func() error {
			defer close(wordsCh)
			count := 0
			for val := range ch {
				line := val.(string)
				words := strings.Split(line, " ")
				for _, w := range words {
					if len(w) < minSize {
						metrics.Inc("skip-small")
						continue
					}
					count++
					wi := wordsInfo{size: len(w), special: strings.Contains(w, "data")}
					if err := flow.Send(ctx, wordsCh, wi); err != nil {
						return errors.Wrapf(err, "failed words handler %d", flow.CID(ctx))
					}
				}
			}
			metrics.Add("words", count)
			log.Printf("words handler completed, id=%03d %d/%d words", flow.CID(ctx), count, metrics.Get("words"))
			return nil
		}
		return wordsCh, wordsFn
	}
}

// sumHandler reduces all inputs with wordsInfo to the final figures and prints it as result
func sumHandler(_ context.Context, ch chan interface{}) (chan interface{}, func() error) {
	nopCh := make(chan interface{})
	sumFn := func() error {
		log.Printf("start sum handler")
		defer close(nopCh)
		words, size, spec := 0, 0, 0
		for val := range ch {
			wi := val.(wordsInfo)
			size += wi.size
			words += 1
			if wi.special {
				spec++
			}
		}
		log.Printf("final result: words=%d, size=%d, spec=%d", words, size, spec)
		return nil
	}
	return nopCh, sumFn
}
