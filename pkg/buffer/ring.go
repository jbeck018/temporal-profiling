// Package buffer provides a lock-free ring buffer for zero-overhead event buffering.
package buffer

import (
	"sync/atomic"
	"time"

	"github.com/temporal-profiling/temporal-profiler/pkg/profiler"
)

// Buffer is the interface for async event buffering.
type Buffer interface {
	// Push adds an event to the buffer (non-blocking).
	// Returns true if the event was added, false if dropped.
	Push(event *profiler.ProfileEvent) bool

	// Consume retrieves up to batchSize events from the buffer.
	Consume(batchSize int) []*profiler.ProfileEvent

	// Stats returns current buffer statistics.
	Stats() Stats
}

// Stats contains buffer statistics.
type Stats struct {
	Size       int64
	Capacity   int64
	Dropped    int64
	Throughput float64
	AvgLatency time.Duration
}

// RingBuffer is a lock-free ring buffer implementation.
// It uses atomic operations to achieve thread-safety without locks,
// making it suitable for the hot path where we need zero overhead.
type RingBuffer struct {
	events   []*profiler.ProfileEvent
	capacity uint64
	mask     uint64

	// Head is the next position to read from
	head uint64
	// Tail is the next position to write to
	tail uint64

	// Statistics
	dropped    uint64
	totalPush  uint64
	totalPop   uint64
	lastUpdate int64
}

// NewRingBuffer creates a new ring buffer with the given capacity.
// Capacity is rounded up to the nearest power of 2 for efficient modulo operations.
func NewRingBuffer(capacity int) *RingBuffer {
	// Round up to power of 2
	size := uint64(1)
	for size < uint64(capacity) {
		size <<= 1
	}

	return &RingBuffer{
		events:     make([]*profiler.ProfileEvent, size),
		capacity:   size,
		mask:       size - 1,
		lastUpdate: time.Now().UnixNano(),
	}
}

// Push adds an event to the buffer without blocking.
// Returns true if successful, false if the buffer is full (event dropped).
func (b *RingBuffer) Push(event *profiler.ProfileEvent) bool {
	for {
		tail := atomic.LoadUint64(&b.tail)
		head := atomic.LoadUint64(&b.head)

		// Check if buffer is full
		if tail-head >= b.capacity {
			atomic.AddUint64(&b.dropped, 1)
			return false
		}

		// Try to claim this slot
		if atomic.CompareAndSwapUint64(&b.tail, tail, tail+1) {
			// Successfully claimed, write the event
			b.events[tail&b.mask] = event
			atomic.AddUint64(&b.totalPush, 1)
			return true
		}
		// CAS failed, another goroutine claimed it, retry
	}
}

// TryPush attempts to push without spinning.
// Returns immediately if CAS fails.
func (b *RingBuffer) TryPush(event *profiler.ProfileEvent) bool {
	tail := atomic.LoadUint64(&b.tail)
	head := atomic.LoadUint64(&b.head)

	// Check if buffer is full
	if tail-head >= b.capacity {
		atomic.AddUint64(&b.dropped, 1)
		return false
	}

	// Try to claim this slot (single attempt)
	if atomic.CompareAndSwapUint64(&b.tail, tail, tail+1) {
		b.events[tail&b.mask] = event
		atomic.AddUint64(&b.totalPush, 1)
		return true
	}

	// CAS failed, don't retry
	return false
}

// Consume retrieves up to batchSize events from the buffer.
// This method is designed for a single consumer.
func (b *RingBuffer) Consume(batchSize int) []*profiler.ProfileEvent {
	head := atomic.LoadUint64(&b.head)
	tail := atomic.LoadUint64(&b.tail)

	// Calculate available events
	available := tail - head
	if available == 0 {
		return nil
	}

	// Limit to batch size
	toConsume := available
	if toConsume > uint64(batchSize) {
		toConsume = uint64(batchSize)
	}

	// Collect events
	events := make([]*profiler.ProfileEvent, toConsume)
	for i := uint64(0); i < toConsume; i++ {
		events[i] = b.events[(head+i)&b.mask]
	}

	// Advance head
	atomic.AddUint64(&b.head, toConsume)
	atomic.AddUint64(&b.totalPop, toConsume)

	return events
}

// Stats returns current buffer statistics.
func (b *RingBuffer) Stats() Stats {
	head := atomic.LoadUint64(&b.head)
	tail := atomic.LoadUint64(&b.tail)
	dropped := atomic.LoadUint64(&b.dropped)
	totalPush := atomic.LoadUint64(&b.totalPush)

	now := time.Now().UnixNano()
	last := atomic.SwapInt64(&b.lastUpdate, now)
	elapsed := float64(now-last) / float64(time.Second)

	var throughput float64
	if elapsed > 0 {
		throughput = float64(totalPush) / elapsed
	}

	return Stats{
		Size:       int64(tail - head),
		Capacity:   int64(b.capacity),
		Dropped:    int64(dropped),
		Throughput: throughput,
	}
}

// Len returns the current number of events in the buffer.
func (b *RingBuffer) Len() int {
	head := atomic.LoadUint64(&b.head)
	tail := atomic.LoadUint64(&b.tail)
	return int(tail - head)
}

// Cap returns the buffer capacity.
func (b *RingBuffer) Cap() int {
	return int(b.capacity)
}

// Dropped returns the total number of dropped events.
func (b *RingBuffer) Dropped() int64 {
	return int64(atomic.LoadUint64(&b.dropped))
}

// Reset clears all statistics (useful for testing).
func (b *RingBuffer) Reset() {
	atomic.StoreUint64(&b.head, 0)
	atomic.StoreUint64(&b.tail, 0)
	atomic.StoreUint64(&b.dropped, 0)
	atomic.StoreUint64(&b.totalPush, 0)
	atomic.StoreUint64(&b.totalPop, 0)
}
