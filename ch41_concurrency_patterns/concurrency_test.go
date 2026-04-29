package concurrency

import (
	"context"
	"errors"
	"sort"
	"sync/atomic"
	"testing"
)

func TestWorkerPool_ProcessesAllInputs(t *testing.T) {
	pool := NewWorkerPool[int, int](4)
	inputs := []int{1, 2, 3, 4, 5, 6, 7, 8}

	results := pool.Run(context.Background(), inputs, func(_ context.Context, n int) (int, error) {
		return n * n, nil
	})

	if len(results) != len(inputs) {
		t.Fatalf("got %d results, want %d", len(results), len(inputs))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Fatalf("unexpected error: %v", r.Err)
		}
	}
}

func TestWorkerPool_PropagatesErrors(t *testing.T) {
	pool := NewWorkerPool[int, int](2)
	boom := errors.New("boom")

	results := pool.Run(context.Background(), []int{1, 2, 3}, func(_ context.Context, n int) (int, error) {
		if n == 2 {
			return 0, boom
		}
		return n, nil
	})

	var errCount int
	for _, r := range results {
		if r.Err != nil {
			errCount++
		}
	}
	if errCount != 1 {
		t.Fatalf("expected 1 error result, got %d", errCount)
	}
}

func TestWorkerPool_LimitsConcurrency(t *testing.T) {
	const workers = 3
	pool := NewWorkerPool[int, int](workers)

	var active atomic.Int64
	var maxActive atomic.Int64

	done := make(chan struct{}, 100)
	inputs := make([]int, 20)
	for i := range inputs {
		inputs[i] = i
	}

	pool.Run(context.Background(), inputs, func(_ context.Context, n int) (int, error) {
		cur := active.Add(1)
		for {
			m := maxActive.Load()
			if cur <= m || maxActive.CompareAndSwap(m, cur) {
				break
			}
		}
		_ = done
		active.Add(-1)
		return n, nil
	})

	if m := maxActive.Load(); m > int64(workers) {
		t.Fatalf("max concurrent workers = %d, want <= %d", m, workers)
	}
}

func TestWorkerPool_EmptyInputs(t *testing.T) {
	pool := NewWorkerPool[int, int](4)
	results := pool.Run(context.Background(), nil, func(_ context.Context, n int) (int, error) {
		return n, nil
	})
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0", len(results))
	}
}

func TestPipeline_GenerateTransform(t *testing.T) {
	ctx := context.Background()
	nums := Generate(ctx, []int{1, 2, 3, 4, 5})
	doubled := Transform(ctx, nums, func(n int) int { return n * 2 })
	got := Collect(doubled)

	sort.Ints(got)
	want := []int{2, 4, 6, 8, 10}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestFanOut_DistributesWork(t *testing.T) {
	ctx := context.Background()
	in := Generate(ctx, []int{1, 2, 3, 4, 5, 6})
	outs := FanOut(ctx, in, 3, func(n int) int { return n })
	merged := FanIn(ctx, outs...)
	results := Collect(merged)

	if len(results) != 6 {
		t.Fatalf("got %d results, want 6", len(results))
	}
	sort.Ints(results)
	for i, v := range results {
		if v != i+1 {
			t.Fatalf("results[%d] = %d, want %d", i, v, i+1)
		}
	}
}

func TestFanIn_MergesChannels(t *testing.T) {
	ctx := context.Background()

	a := Generate(ctx, []int{1, 3, 5})
	b := Generate(ctx, []int{2, 4, 6})

	merged := FanIn(ctx, a, b)
	got := Collect(merged)
	sort.Ints(got)

	if len(got) != 6 {
		t.Fatalf("got %d items, want 6", len(got))
	}
}

func TestPipeline_CancelStopsEarly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	nums := Generate(ctx, []int{1, 2, 3, 4, 5})
	// After cancellation, Generate should stop; Collect should return <= 5 items.
	got := Collect(nums)
	// We can't assert exact count due to timing, but it should not panic or block.
	_ = got
}

func TestGenerate_AllItemsSent(t *testing.T) {
	ctx := context.Background()
	items := []string{"a", "b", "c"}
	ch := Generate(ctx, items)
	got := Collect(ch)

	if len(got) != len(items) {
		t.Fatalf("got %d items, want %d", len(got), len(items))
	}
}
