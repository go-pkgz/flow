package pool

// Option func type
type Option func(p *Workers)

// ChunkFn functional option defines chunk func distributing records to particular workers.
// The function should return key string identifying the record.
// Record with a given key string guaranteed to be processed by the same worker.
func ChunkFn(chunkFn func(val interface{}) string) Option {
	return func(p *Workers) {
		p.chunkFn = chunkFn
	}
}

// ResChanSize sets size of response's channel buffer
func ResChanSize(size int) Option {
	return func(p *Workers) {
		if size >= 0 {
			p.resChanSize = size
		}
	}
}

// WorkerChanSize sets size of worker channel(s)
func WorkerChanSize(size int) Option {
	return func(p *Workers) {
		if size >= 0 {
			p.workerChanSize = size
		}
	}
}

// Batch sets batch size to collect incoming records in a buffer before sending to workers
func Batch(size int) Option {
	return func(p *Workers) {
		if size >= 1 {
			p.batchSize = size
		}
	}
}

// ContinueOnError change default early termination on the first error and continue after error
func ContinueOnError(p *Workers) {
	p.continueOnError = true
}

// OnCompletion set function called on completion for each worker id
func OnCompletion(completeFn CompleteFn) Option {
	return func(p *Workers) {
		p.completeFn = completeFn
	}
}
