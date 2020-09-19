package pool

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPool(t *testing.T) {

	poolTest := func(poolSize, batchSize int, chunks bool, workerChSize, resChSize int) {
		type inpRec struct {
			Num int
			Fld string
		}

		worker := func(ctx context.Context, v interface{}, resCh chan Response, store WorkerStore) error {
			rec := v.(inpRec)
			if rec.Num%10 == 0 {
				err := Send(ctx, resCh, Response{Value: fmt.Sprintf("%s-%03d", rec.Fld, rec.Num)})
				require.NoError(t, err)
			}
			Metrics(ctx).Inc("count")
			return nil
		}

		var opts []Option
		if chunks {
			opts = append(opts, ChunkFn(func(v interface{}) string {
				return v.(inpRec).Fld
			}))
		}
		opts = append(opts, Batch(batchSize), ResChanSize(resChSize), WorkerChanSize(workerChSize))
		p := New(poolSize, worker, opts...)

		ctx := context.Background()
		ch := p.Go(ctx)
		go func() {
			for i := 0; i < 1000; i++ {
				p.Submit(inpRec{Num: i, Fld: fmt.Sprintf("val-%03d", i)})
			}
			p.Close()
		}()

		n := 0
		var res []string
		for v := range ch {
			log.Printf("%+v", v)
			res = append(res, v.Value.(string))
			n++
		}
		require.Equal(t, 100, n)
		assert.Equal(t, 1000, p.Metrics().Get("count"))
		sort.Strings(res)
		require.Equal(t, []string{"val-000-000", "val-010-010", "val-020-020", "val-030-030", "val-040-040", "val-050-050",
			"val-060-060", "val-070-070", "val-080-080", "val-090-090", "val-100-100", "val-110-110", "val-120-120",
			"val-130-130", "val-140-140", "val-150-150", "val-160-160", "val-170-170", "val-180-180", "val-190-190",
			"val-200-200", "val-210-210", "val-220-220", "val-230-230", "val-240-240", "val-250-250", "val-260-260",
			"val-270-270", "val-280-280", "val-290-290", "val-300-300", "val-310-310", "val-320-320", "val-330-330",
			"val-340-340", "val-350-350", "val-360-360", "val-370-370", "val-380-380", "val-390-390", "val-400-400",
			"val-410-410", "val-420-420", "val-430-430", "val-440-440", "val-450-450", "val-460-460", "val-470-470",
			"val-480-480", "val-490-490", "val-500-500", "val-510-510", "val-520-520", "val-530-530", "val-540-540",
			"val-550-550", "val-560-560", "val-570-570", "val-580-580", "val-590-590", "val-600-600", "val-610-610",
			"val-620-620", "val-630-630", "val-640-640", "val-650-650", "val-660-660", "val-670-670", "val-680-680",
			"val-690-690", "val-700-700", "val-710-710", "val-720-720", "val-730-730", "val-740-740", "val-750-750",
			"val-760-760", "val-770-770", "val-780-780", "val-790-790", "val-800-800", "val-810-810", "val-820-820",
			"val-830-830", "val-840-840", "val-850-850", "val-860-860", "val-870-870", "val-880-880", "val-890-890",
			"val-900-900", "val-910-910", "val-920-920", "val-930-930", "val-940-940", "val-950-950", "val-960-960",
			"val-970-970", "val-980-980", "val-990-990"}, res)
	}

	tbl := []struct {
		poolSize, batchSize     int
		workerChSize, resChSize int
	}{
		{poolSize: 1, batchSize: 1, workerChSize: 1, resChSize: 1},
		{poolSize: 1, batchSize: 1, workerChSize: 0, resChSize: 0},
		{poolSize: 1, batchSize: 10, workerChSize: 10, resChSize: 10},
		{poolSize: 1, batchSize: 12, workerChSize: 1, resChSize: 10},
		{poolSize: 1, batchSize: 100, workerChSize: 10, resChSize: 1},
		{poolSize: 1, batchSize: 123, workerChSize: 1, resChSize: 1},
		{poolSize: 3, batchSize: 1, workerChSize: 1, resChSize: 1},
		{poolSize: 3, batchSize: 5, workerChSize: 1, resChSize: 1},
		{poolSize: 3, batchSize: 10, workerChSize: 1, resChSize: 1},
		{poolSize: 3, batchSize: 100, workerChSize: 1, resChSize: 1},
		{poolSize: 3, batchSize: 105, workerChSize: 1, resChSize: 1},
		{poolSize: 8, batchSize: 1, workerChSize: 1, resChSize: 1},
		{poolSize: 8, batchSize: 10, workerChSize: 1, resChSize: 1},
		{poolSize: 8, batchSize: 11, workerChSize: 1, resChSize: 1},
		{poolSize: 8, batchSize: 50, workerChSize: 1, resChSize: 1},
		{poolSize: 8, batchSize: 90, workerChSize: 1, resChSize: 1},
		{poolSize: 8, batchSize: 100, workerChSize: 1, resChSize: 1},
		{poolSize: 8, batchSize: 345, workerChSize: 1, resChSize: 1},
		{poolSize: 8, batchSize: 345, workerChSize: 0, resChSize: 0},
		{poolSize: 0, batchSize: 345, workerChSize: 0, resChSize: 0},
	}

	for i, tt := range tbl {
		t.Run(fmt.Sprintf("%d %d:%d", i, tt.poolSize, tt.batchSize), func(t *testing.T) {
			poolTest(tt.poolSize, tt.batchSize, true, tt.workerChSize, tt.resChSize)
			poolTest(tt.poolSize, tt.batchSize, false, tt.workerChSize, tt.resChSize)
		})
	}
}

func TestPoolWithStore(t *testing.T) {

	worker := func(ctx context.Context, v interface{}, resCh chan Response, store WorkerStore) error {
		store.Set("counter", store.GetInt("counter")+1)
		Metrics(ctx).Add("c", 1)
		return nil
	}

	var counts int64
	completion := func(ctx context.Context, respCh chan Response, store WorkerStore) error {
		count := store.GetInt("counter")
		cc := atomic.AddInt64(&counts, int64(count))
		assert.True(t, count > 0)
		log.Printf("%d %d [%d]", count, cc, WorkerID(ctx))
		return nil
	}

	ctx := context.Background()

	p := New(7, worker, OnCompletion(completion))
	p.Go(ctx)

	go func() {
		for i := 0; i < 1000; i++ {
			p.Submit("line")
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(5)))
		}
		p.Close()
	}()

	shortCtx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	assert.EqualError(t, p.Wait(shortCtx), "context deadline exceeded")

	assert.NoError(t, p.Wait(context.Background()))
	assert.Equal(t, int64(1000), counts)
	assert.Equal(t, 1000, p.Metrics().Get("c"))
}

func TestPoolCanceled(t *testing.T) {
	worker := func(ctx context.Context, v interface{}, resCh chan Response, store WorkerStore) error {
		time.Sleep(100 * time.Millisecond)
		resCh <- Response{Value: v}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
	defer cancel()

	p := New(7, worker)

	ch := p.Go(ctx)

	go func() {
		for i := 0; i < 1000; i++ {
			p.Submit("line")
			time.Sleep(time.Millisecond * 100)
		}
		p.Close()
	}()

	n := 0
	var err error
	for v := range ch {
		if v.Error != nil {
			err = v.Error
		}
		n++
	}
	assert.True(t, n < 1000)
	assert.EqualError(t, err, "context deadline exceeded")
	assert.EqualError(t, ctx.Err(), context.DeadlineExceeded.Error())
}

func TestPoolError(t *testing.T) {
	worker := func(_ context.Context, v interface{}, resCh chan Response, store WorkerStore) error {
		if rand.Intn(10) > 5 {
			return errors.New("some error")
		}
		resCh <- Response{Value: v}
		return nil
	}

	p := New(7, worker, Batch(5), ResChanSize(1))

	ch := p.Go(context.Background())

	go func() {
		for i := 0; i < 1000; i++ {
			p.Submit("line")
		}
		p.Close()
	}()

	vals, err := p.ReadAll(ch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "some error")
	assert.True(t, len(vals) < 1000)

	ch1 := p.Go(context.Background())
	r := <-ch1
	assert.EqualError(t, r.Error, "workers poll already activated")
}

func TestPoolErrorContinue(t *testing.T) {
	var c int64
	worker := func(_ context.Context, v interface{}, resCh chan Response, store WorkerStore) error {
		atomic.AddInt64(&c, 1)
		var e error
		if rand.Intn(10) > 5 {
			e = errors.New("some error")
		}
		resCh <- Response{Value: v, Error: e}
		return nil
	}

	p := New(7, worker, Batch(5), ResChanSize(1), ContinueOnError)

	ch := p.Go(context.Background())

	go func() {
		for i := 0; i < 1000; i++ {
			p.Submit("line")
		}
		p.Close()
	}()

	vals, err := p.ReadAll(ch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "some error")
	assert.Equal(t, 1000, len(vals))

	assert.Equal(t, int64(1000), atomic.LoadInt64(&c))
	ch1 := p.Go(context.Background())
	r := <-ch1
	assert.EqualError(t, r.Error, "workers poll already activated")
}

func TestWorkers_SubmitWithChunks(t *testing.T) {
	tbl := []struct {
		inp      string
		poolSize int
		buf      [][]interface{}
	}{
		{"test", 7, [][]interface{}{{"test"}, {}, {}, {}, {}, {}, {}}},
		{"test2", 7, [][]interface{}{{}, {}, {}, {"test2"}, {}, {}, {}}},
		{"test3", 7, [][]interface{}{{}, {}, {"test3"}, {}, {}, {}, {}}},
		{"test123", 7, [][]interface{}{{}, {}, {"test123"}, {}, {}, {}, {}}},
		{"test123", 1, [][]interface{}{{"test123"}}},
		{"test12345", 1, [][]interface{}{{"test12345"}}},
		{"zzzz", 2, [][]interface{}{{"zzzz"}, {}}},
		{"xxxx", 2, [][]interface{}{{}, {"xxxx"}}},
	}

	wk := func(ctx context.Context, inpRec interface{}, respCh chan Response, store WorkerStore) error {
		return nil
	}

	for i, tt := range tbl {

		p := New(tt.poolSize, wk, Batch(5), ChunkFn(func(val interface{}) string {
			return val.(string) + "$"
		}))

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			p.Submit(tt.inp)
			assert.Equal(t, tt.buf, p.buf)
		})
	}

}

func TestWorkers_SubmitNoChunkFn(t *testing.T) {

	wk := func(ctx context.Context, inpRec interface{}, respCh chan Response, store WorkerStore) error {
		return nil
	}

	p := New(8, wk, Batch(1000))

	for i := 0; i < 1000; i++ {
		p.Submit("something " + strconv.Itoa(i))
	}

	tot := 0
	for j := 0; j < 8; j++ {
		tot += len(p.buf[j])
		assert.True(t, len(p.buf[j]) > 0 && len(p.buf[j]) < 1000)
	}
	assert.Equal(t, 1000, tot)

}

// illustrates basic use of workers pool
func ExampleWorkers_basic() {

	workerFn := func(ctx context.Context, inpRec interface{}, respCh chan Response, store WorkerStore) error {
		v, ok := inpRec.(string)
		if !ok {
			return errors.New("incorrect input type")
		}
		// do something with v
		res := strings.ToUpper(v)

		// send response
		return Send(ctx, respCh, Response{Value: res})
	}

	p := New(8, workerFn) // create workers pool
	resp := p.Go(context.Background())

	// send some records in
	go func() {
		p.Submit("rec1")
		p.Submit("rec2")
		p.Submit("rec3")
		p.Close() // all records sent
	}()

	// consume results
	recs, err := p.ReadAll(resp)
	log.Printf("%+v, %v", recs, err)
}

// illustrates use of workers pool with all options
func ExampleWorkers_withOptions() {

	workerFn := func(ctx context.Context, inpRec interface{}, respCh chan Response, store WorkerStore) error {
		v, ok := inpRec.(string)
		if !ok {
			return errors.New("incorrect input type")
		}
		// do something with v
		res := strings.ToUpper(v)

		// update metrics
		m := Metrics(ctx)
		m.Inc("count")

		// send response
		return Send(ctx, respCh, Response{Value: res})
	}

	// create workers pool with chunks and batch mode. ChunkFn used to detect worker and guaranteed to send same chunk
	// to the same worker. This is important for stateful workers. Batch sets the size of internal buffer collecting records
	// internally before sending them to worker.
	p := New(8, workerFn, Batch(10), ResChanSize(5), WorkerChanSize(2), ChunkFn(func(val interface{}) string {
		v := val.(string)
		return v[:4] // chunks by 4chars prefix
	}))
	resp := p.Go(context.Background())

	// send some records in
	go func() {
		p.Submit("rec1")
		p.Submit("rec2")
		p.Submit("rec3")
		p.Close() // all records sent
	}()

	// consume results in streaming mode
	for r := range resp {
		if r.Error != nil {
			panic(r.Error)
		}
		log.Printf("%v", r.Value)
	}

	// show metrics
	log.Printf("metrics: %s", p.Metrics())
}
