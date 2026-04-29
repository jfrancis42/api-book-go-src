package concurrency

import (
	"context"
	"sync"
)

// WorkerPool runs jobs concurrently with a fixed number of workers.
// Results are returned in completion order, not submission order.
type WorkerPool[In, Out any] struct {
	workers int
}

// Result pairs each output with its error.
type Result[Out any] struct {
	Value Out
	Err   error
}

func NewWorkerPool[In, Out any](workers int) *WorkerPool[In, Out] {
	return &WorkerPool[In, Out]{workers: workers}
}

// Run processes all inputs concurrently and returns results as a slice.
// Cancelling ctx stops accepting new work but in-flight jobs complete.
func (p *WorkerPool[In, Out]) Run(ctx context.Context, inputs []In, fn func(context.Context, In) (Out, error)) []Result[Out] {
	jobs := make(chan In, len(inputs))
	for _, in := range inputs {
		jobs <- in
	}
	close(jobs)

	results := make(chan Result[Out], len(inputs))
	var wg sync.WaitGroup
	for range p.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for in := range jobs {
				if ctx.Err() != nil {
					break
				}
				v, err := fn(ctx, in)
				results <- Result[Out]{Value: v, Err: err}
			}
		}()
	}

	wg.Wait()
	close(results)

	out := make([]Result[Out], 0, len(inputs))
	for r := range results {
		out = append(out, r)
	}
	return out
}

// Pipeline connects a generator, a transform stage, and a sink using channels.
// Each stage runs in its own goroutine; the sink collects final results.

// Generate sends each item into a channel and closes it when done.
func Generate[T any](ctx context.Context, items []T) <-chan T {
	out := make(chan T)
	go func() {
		defer close(out)
		for _, item := range items {
			select {
			case out <- item:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// Transform reads from in, applies fn, and sends to the returned channel.
func Transform[In, Out any](ctx context.Context, in <-chan In, fn func(In) Out) <-chan Out {
	out := make(chan Out)
	go func() {
		defer close(out)
		for v := range in {
			select {
			case out <- fn(v):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// FanOut distributes work from one input channel across n workers,
// each applying fn. All n output channels are returned.
func FanOut[In, Out any](ctx context.Context, in <-chan In, n int, fn func(In) Out) []<-chan Out {
	outs := make([]<-chan Out, n)
	for i := range n {
		ch := make(chan Out)
		outs[i] = ch
		go func(out chan Out) {
			defer close(out)
			for v := range in {
				select {
				case out <- fn(v):
				case <-ctx.Done():
					return
				}
			}
		}(ch)
	}
	return outs
}

// FanIn merges multiple input channels into one output channel.
// The output channel is closed when all inputs are exhausted.
func FanIn[T any](ctx context.Context, ins ...<-chan T) <-chan T {
	out := make(chan T)
	var wg sync.WaitGroup
	for _, in := range ins {
		wg.Add(1)
		go func(ch <-chan T) {
			defer wg.Done()
			for v := range ch {
				select {
				case out <- v:
				case <-ctx.Done():
					return
				}
			}
		}(in)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// Collect drains a channel into a slice.
func Collect[T any](ch <-chan T) []T {
	var out []T
	for v := range ch {
		out = append(out, v)
	}
	return out
}
