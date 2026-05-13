package listen

import (
	"context"
	"sync"
	"time"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
)

type serviceBuffer struct {
	items []*messagesv1.WriteRequest
	timer *time.Timer
}

// Batcher groups incoming WriteRequests by service and flushes them when a batch
// reaches batchSize or flushInterval elapses since the first message in the batch.
type Batcher struct {
	mu            sync.Mutex
	wg            sync.WaitGroup
	buffers       map[string]*serviceBuffer
	batchSize     int
	flushInterval time.Duration
	flush         func(ctx context.Context, service string, items []*messagesv1.WriteRequest) error
}

// NewBatcher creates a new Batcher.
func NewBatcher(
	batchSize int,
	flushInterval time.Duration,
	flush func(ctx context.Context, service string, items []*messagesv1.WriteRequest) error,
) *Batcher {
	return &Batcher{
		buffers:       make(map[string]*serviceBuffer),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		flush:         flush,
	}
}

// Add appends msg to the buffer for service, triggering a flush if the batch is full.
func (b *Batcher) Add(service string, msg *messagesv1.WriteRequest) {
	b.mu.Lock()
	buf, ok := b.buffers[service]
	if !ok {
		buf = &serviceBuffer{}
		b.buffers[service] = buf
		buf.timer = time.AfterFunc(b.flushInterval, func() {
			b.timerFlush(service)
		})
	}
	buf.items = append(buf.items, msg)

	if len(buf.items) >= b.batchSize {
		items := buf.items
		buf.timer.Stop()
		delete(b.buffers, service)
		b.wg.Add(1)
		b.mu.Unlock()
		go func() {
			defer b.wg.Done()
			_ = b.flush(context.Background(), service, items)
		}()
		return
	}
	b.mu.Unlock()
}

// Shutdown flushes all pending buffers using the provided context, then waits for
// all in-flight flushes (including timer-triggered ones) to complete.
func (b *Batcher) Shutdown(ctx context.Context) {
	b.mu.Lock()
	type pending struct {
		service string
		items   []*messagesv1.WriteRequest
	}
	var all []pending
	for service, buf := range b.buffers {
		buf.timer.Stop()
		if len(buf.items) > 0 {
			b.wg.Add(1)
			all = append(all, pending{service, buf.items})
		}
	}
	clear(b.buffers)
	b.mu.Unlock()

	for _, p := range all {
		p := p
		go func() {
			defer b.wg.Done()
			_ = b.flush(ctx, p.service, p.items)
		}()
	}

	b.wg.Wait()
}

func (b *Batcher) timerFlush(service string) {
	b.mu.Lock()
	buf, ok := b.buffers[service]
	if !ok || len(buf.items) == 0 {
		b.mu.Unlock()
		return
	}
	items := buf.items
	delete(b.buffers, service)
	b.wg.Add(1)
	b.mu.Unlock()

	go func() {
		defer b.wg.Done()
		_ = b.flush(context.Background(), service, items)
	}()
}
