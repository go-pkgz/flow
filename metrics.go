package flow

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Metrics collects and increments any int numbers, can be used to safely count things in flow.
// Flow creates empty metrics object and puts it to the context. Consumers (user provided handlers) can
// retrieve it directly from the context by doing metrics := ctx.Value(flow.MetricsContextKey).(*flow.Metrics)
// Provided GetMetrics does exactly the same thing.
// Flow also has a helper method Metrics() to retrieve metrics from ctx.
type Metrics struct {
	startTime time.Time

	userLock sync.RWMutex
	userData map[string]int
}

type contextKey string

// MetricsContextKey used as metrics key in ctx
const MetricsContextKey contextKey = "metrics"

// CidContextKey used as concurrentID key in ctx. It doesn't do anything magical, just represents a special
// case metric set by flow internally. Flow sets cid to indicate which of parallel handlers is processing data subset.
const CidContextKey contextKey = "cid"

// NewMetrics makes thread-safe map to collect any counts/metrics
func NewMetrics() *Metrics {
	return &Metrics{startTime: time.Now(), userData: map[string]int{}}
}

// GetMetrics from context
func GetMetrics(ctx context.Context) *Metrics {
	res, ok := ctx.Value(MetricsContextKey).(*Metrics)
	if !ok {
		return NewMetrics()
	}
	return res
}

// Add increments value for given key and returns new value
func (m *Metrics) Add(key string, delta int) int {
	m.userLock.Lock()
	defer m.userLock.Unlock()
	m.userData[key] += delta
	return m.userData[key]
}

// Inc increments value for given key by one
func (m *Metrics) Inc(key string) int {
	return m.Add(key, 1)
}

// Set value for given key
func (m *Metrics) Set(key string, val int) {
	m.userLock.Lock()
	defer m.userLock.Unlock()
	m.userData[key] = val
}

// Get returns value for given key
func (m *Metrics) Get(key string) int {
	m.userLock.RLock()
	defer m.userLock.RUnlock()

	return m.userData[key]
}

// String returns sorted key:val string representation of metrics and adds duration
func (m *Metrics) String() string {
	duration := time.Since(m.startTime)

	m.userLock.RLock()
	defer m.userLock.RUnlock()

	sortedKeys := func() (res []string) {
		for k := range m.userData {
			res = append(res, k)
		}
		sort.Strings(res)
		return res
	}()

	var udata = make([]string, len(sortedKeys))
	for i, k := range sortedKeys {
		udata[i] = fmt.Sprintf("%s:%d", k, m.userData[k])
	}
	um := ""
	if len(udata) > 0 {
		um = fmt.Sprintf("[%s]", strings.Join(udata, ", "))
	}
	return fmt.Sprintf("%v %s", duration, um)
}
