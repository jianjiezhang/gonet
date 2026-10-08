package algo

import (
	"errors"
	"fmt"
	"slices"
	"testing"
)

type minInt struct{}

func (minInt) Less(a, b int) bool { return a < b }

type maxInt struct{}

func (maxInt) Less(a, b int) bool { return a > b }

func mustHeap[T any](t *testing.T, order Order[T]) *Heap[T] {
	t.Helper()
	h, err := New(order)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func checkHeap[T any](t *testing.T, h *Heap[T]) {
	t.Helper()
	if h.Len() != len(h.data) || len(h.data) != len(h.node) || len(h.pos) != len(h.data) {
		t.Fatalf("len mismatch data=%d node=%d pos=%d Len=%d", len(h.data), len(h.node), len(h.pos), h.Len())
	}
	for i, n := range h.node {
		p, ok := h.pos[n]
		if !ok || p != i {
			t.Fatalf("handle %d pos want %d got %d ok=%v", n, i, p, ok)
		}
	}
	for i := range h.data {
		l, r := 2*i+1, 2*i+2
		if l < len(h.data) && h.lessAt(l, i) {
			t.Fatalf("heap broken at %d child %d: %+v", i, l, h.data)
		}
		if r < len(h.data) && h.lessAt(r, i) {
			t.Fatalf("heap broken at %d child %d: %+v", i, r, h.data)
		}
	}
}

func popAll[T any](t *testing.T, h *Heap[T]) []T {
	t.Helper()
	out := make([]T, 0, h.Len())
	for h.Len() > 0 {
		checkHeap(t, h)
		v, err := h.Pop()
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	if _, err := h.Pop(); !errors.Is(err, ErrEmptyHeap) {
		t.Fatalf("empty Pop: %v", err)
	}
	return out
}

func TestNewNilOrder(t *testing.T) {
	_, err := New[int](nil)
	if !errors.Is(err, ErrNilOrder) {
		t.Fatalf("got %v", err)
	}
}

func TestEmptyHeap(t *testing.T) {
	h := mustHeap(t, minInt{})
	if h.Len() != 0 {
		t.Fatalf("Len=%d", h.Len())
	}
	if _, err := h.Peek(); !errors.Is(err, ErrEmptyHeap) {
		t.Fatalf("Peek: %v", err)
	}
	if _, err := h.Pop(); !errors.Is(err, ErrEmptyHeap) {
		t.Fatalf("Pop: %v", err)
	}
	if err := h.Update(1, 0); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("Update: %v", err)
	}
	if err := h.Remove(1); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := h.Get(1); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("Get: %v", err)
	}
	if err := h.Update(0, 1); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("Update 0: %v", err)
	}
}

func TestNilHeap(t *testing.T) {
	var h *Heap[int]
	if h.Len() != 0 {
		t.Fatalf("Len=%d", h.Len())
	}
	if _, err := h.Peek(); !errors.Is(err, ErrEmptyHeap) {
		t.Fatalf("Peek: %v", err)
	}
	if _, err := h.Pop(); !errors.Is(err, ErrEmptyHeap) {
		t.Fatalf("Pop: %v", err)
	}
	if err := h.Update(1, 1); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("Update: %v", err)
	}
}

func TestSingleElement(t *testing.T) {
	h := mustHeap(t, minInt{})
	n := h.Push(42)
	checkHeap(t, h)
	if h.Len() != 1 {
		t.Fatal("Len")
	}
	v, err := h.Peek()
	if err != nil || v != 42 {
		t.Fatalf("Peek %d %v", v, err)
	}
	if err := h.Update(n, 7); err != nil {
		t.Fatal(err)
	}
	checkHeap(t, h)
	got, err := h.Get(n)
	if err != nil || got != 7 {
		t.Fatalf("Get %d %v", got, err)
	}
	v, err = h.Pop()
	if err != nil || v != 7 {
		t.Fatalf("Pop %d %v", v, err)
	}
	if h.Len() != 0 {
		t.Fatal("not empty")
	}
}

func TestMinHeapSort(t *testing.T) {
	h := mustHeap(t, minInt{})
	in := []int{5, 1, 9, 3, 3, 0, -4, 8, 1}
	for _, v := range in {
		h.Push(v)
		checkHeap(t, h)
	}
	got := popAll(t, h)
	want := slices.Clone(in)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestMaxHeapSort(t *testing.T) {
	h := mustHeap(t, maxInt{})
	in := []int{5, 1, 9, 3, 3, 0, -4, 8, 1}
	for _, v := range in {
		h.Push(v)
		checkHeap(t, h)
	}
	got := popAll(t, h)
	want := slices.Clone(in)
	slices.Sort(want)
	slices.Reverse(want)
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestLessFuncMax(t *testing.T) {
	h := mustHeap(t, LessFunc[int](func(a, b int) bool { return a > b }))
	h.Push(1)
	h.Push(9)
	h.Push(4)
	v, err := h.Peek()
	if err != nil || v != 9 {
		t.Fatalf("got %d err=%v", v, err)
	}
}

func TestDuplicates(t *testing.T) {
	h := mustHeap(t, minInt{})
	for i := 0; i < 8; i++ {
		h.Push(3)
	}
	checkHeap(t, h)
	got := popAll(t, h)
	for _, v := range got {
		if v != 3 {
			t.Fatalf("got %v", got)
		}
	}
}

func TestPushAfterEmpty(t *testing.T) {
	h := mustHeap(t, minInt{})
	h.Push(2)
	h.Push(1)
	_, _ = h.Pop()
	_, _ = h.Pop()
	n := h.Push(5)
	checkHeap(t, h)
	if err := h.Update(n, 0); err != nil {
		t.Fatal(err)
	}
	v, err := h.Pop()
	if err != nil || v != 0 {
		t.Fatalf("got %d %v", v, err)
	}
}

func TestUpdateSwimMin(t *testing.T) {
	h := mustHeap(t, minInt{})
	a := h.Push(10)
	h.Push(20)
	h.Push(30)
	if err := h.Update(a, 1); err != nil {
		t.Fatal(err)
	}
	checkHeap(t, h)
	v, err := h.Peek()
	if err != nil || v != 1 {
		t.Fatalf("got %d err=%v", v, err)
	}
}

func TestUpdateSinkMin(t *testing.T) {
	h := mustHeap(t, minInt{})
	a := h.Push(1)
	h.Push(2)
	h.Push(3)
	if err := h.Update(a, 100); err != nil {
		t.Fatal(err)
	}
	checkHeap(t, h)
	if got := popAll(t, h); !slices.Equal(got, []int{2, 3, 100}) {
		t.Fatalf("got %v", got)
	}
}

func TestUpdateSameValue(t *testing.T) {
	h := mustHeap(t, minInt{})
	n := h.Push(4)
	h.Push(5)
	h.Push(6)
	if err := h.Update(n, 4); err != nil {
		t.Fatal(err)
	}
	checkHeap(t, h)
	v, err := h.Peek()
	if err != nil || v != 4 {
		t.Fatalf("got %d %v", v, err)
	}
}

func TestUpdateMaxHeap(t *testing.T) {
	h := mustHeap(t, maxInt{})
	n := h.Push(5)
	h.Push(3)
	h.Push(1)
	if err := h.Update(n, 0); err != nil {
		t.Fatal(err)
	}
	checkHeap(t, h)
	if got := popAll(t, h); !slices.Equal(got, []int{3, 1, 0}) {
		t.Fatalf("got %v", got)
	}

	h = mustHeap(t, maxInt{})
	h.Push(3)
	h.Push(2)
	leaf := h.Push(1)
	if err := h.Update(leaf, 9); err != nil {
		t.Fatal(err)
	}
	checkHeap(t, h)
	v, err := h.Peek()
	if err != nil || v != 9 {
		t.Fatalf("got %d %v", v, err)
	}
}

func TestUpdateManyTimes(t *testing.T) {
	h := mustHeap(t, minInt{})
	nodes := make([]Node, 0, 6)
	for i := 10; i <= 15; i++ {
		nodes = append(nodes, h.Push(i))
	}
	for i, n := range nodes {
		if err := h.Update(n, i); err != nil {
			t.Fatal(err)
		}
		checkHeap(t, h)
	}
	if got := popAll(t, h); !slices.Equal(got, []int{0, 1, 2, 3, 4, 5}) {
		t.Fatalf("got %v", got)
	}
}

func TestUpdateThenPopAll(t *testing.T) {
	h := mustHeap(t, minInt{})
	n1 := h.Push(4)
	n2 := h.Push(8)
	n3 := h.Push(6)
	if err := h.Update(n2, 1); err != nil {
		t.Fatal(err)
	}
	if err := h.Update(n1, 9); err != nil {
		t.Fatal(err)
	}
	if err := h.Update(n3, 3); err != nil {
		t.Fatal(err)
	}
	checkHeap(t, h)
	if got := popAll(t, h); !slices.Equal(got, []int{1, 3, 9}) {
		t.Fatalf("got %v", got)
	}
}

func TestRemoveRootMiddleLast(t *testing.T) {
	h := mustHeap(t, minInt{})
	n1 := h.Push(1)
	n2 := h.Push(2)
	n3 := h.Push(3)
	n4 := h.Push(4)
	if err := h.Remove(n1); err != nil {
		t.Fatal(err)
	}
	checkHeap(t, h)
	if err := h.Remove(n3); err != nil {
		t.Fatal(err)
	}
	checkHeap(t, h)
	if err := h.Remove(n4); err != nil {
		t.Fatal(err)
	}
	checkHeap(t, h)
	v, err := h.Pop()
	if err != nil || v != 2 {
		t.Fatalf("got %d %v", v, err)
	}
	_ = n2
}

func TestRemoveLastRemaining(t *testing.T) {
	h := mustHeap(t, minInt{})
	n := h.Push(9)
	if err := h.Remove(n); err != nil {
		t.Fatal(err)
	}
	if h.Len() != 0 {
		t.Fatal("Len")
	}
	if err := h.Remove(n); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("stale Remove: %v", err)
	}
}

func TestStaleNodeAfterPop(t *testing.T) {
	h := mustHeap(t, minInt{})
	n := h.Push(1)
	h.Push(2)
	if _, err := h.Pop(); err != nil {
		t.Fatal(err)
	}
	if err := h.Update(n, 0); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("got %v", err)
	}
}

func TestGetAfterUpdate(t *testing.T) {
	h := mustHeap(t, minInt{})
	n := h.Push(8)
	h.Push(1)
	if err := h.Update(n, 0); err != nil {
		t.Fatal(err)
	}
	v, err := h.Get(n)
	if err != nil || v != 0 {
		t.Fatalf("got %d %v", v, err)
	}
	p, err := h.Peek()
	if err != nil || p != 0 {
		t.Fatalf("Peek %d %v", p, err)
	}
}

func TestDistinctHandles(t *testing.T) {
	h := mustHeap(t, minInt{})
	a := h.Push(1)
	b := h.Push(1)
	if a == 0 || a == b {
		t.Fatalf("handles %d %d", a, b)
	}
}

func TestMinMaxMixedValues(t *testing.T) {
	in := []int{-100, 0, 100, -1, 50}
	for name, order := range map[string]Order[int]{"min": minInt{}, "max": maxInt{}} {
		t.Run(name, func(t *testing.T) {
			h := mustHeap(t, order)
			var nodes []Node
			for _, v := range in {
				nodes = append(nodes, h.Push(v))
			}
			checkHeap(t, h)
			if err := h.Update(nodes[0], 7); err != nil {
				t.Fatal(err)
			}
			checkHeap(t, h)
			got := popAll(t, h)
			if len(got) != len(in) {
				t.Fatalf("len %d", len(got))
			}
			for i := 1; i < len(got); i++ {
				if name == "min" && got[i] < got[i-1] {
					t.Fatalf("not sorted asc %v", got)
				}
				if name == "max" && got[i] > got[i-1] {
					t.Fatalf("not sorted desc %v", got)
				}
			}
		})
	}
}

func TestUpdateInvalid(t *testing.T) {
	h := mustHeap(t, minInt{})
	h.Push(1)
	if err := h.Update(0, 1); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("got %v", err)
	}
	if err := h.Update(99, 1); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("got %v", err)
	}
}

func ExampleHeap_min() {
	h, _ := New[int](minInt{})
	h.Push(5)
	h.Push(1)
	h.Push(3)
	for h.Len() > 0 {
		v, _ := h.Pop()
		fmt.Println(v)
	}
	// Output:
	// 1
	// 3
	// 5
}
