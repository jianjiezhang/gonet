package algo

import "errors"

var (
	ErrNilOrder    = errors.New("gonet/algo: 比较器不能为 nil")
	ErrInvalidNode = errors.New("gonet/algo: 节点不存在")
	ErrEmptyHeap   = errors.New("gonet/algo: 堆为空")
)

// Order 决定堆序。Less(a,b)==true 表示 a 比 b 更靠近根（先被 Pop）。
// 最小堆：return a < b；最大堆：return a > b。
type Order[T any] interface {
	Less(a, b T) bool
}

// LessFunc 把函数当成 Order。
type LessFunc[T any] func(a, b T) bool

func (f LessFunc[T]) Less(a, b T) bool {
	return f(a, b)
}

// Node 是 Push 返回的句柄，Update / Remove 用它定位节点。
type Node uint64

// Heap 是带句柄的二叉堆。Update 改值后会上浮或下沉，恢复堆序。
type Heap[T any] struct {
	order Order[T]
	data  []T
	node  []Node
	pos   map[Node]int
	next  Node
}

func New[T any](order Order[T]) (*Heap[T], error) {
	if order == nil {
		return nil, ErrNilOrder
	}
	return &Heap[T]{
		order: order,
		pos:   make(map[Node]int),
	}, nil
}

func (h *Heap[T]) Len() int {
	if h == nil {
		return 0
	}
	return len(h.data)
}

func (h *Heap[T]) Push(v T) Node {
	h.next++
	n := h.next
	h.data = append(h.data, v)
	h.node = append(h.node, n)
	h.pos[n] = len(h.data) - 1
	h.up(len(h.data) - 1)
	return n
}

func (h *Heap[T]) Peek() (T, error) {
	var zero T
	if h == nil || len(h.data) == 0 {
		return zero, ErrEmptyHeap
	}
	return h.data[0], nil
}

func (h *Heap[T]) Pop() (T, error) {
	var zero T
	if h == nil || len(h.data) == 0 {
		return zero, ErrEmptyHeap
	}
	v := h.data[0]
	h.removeAt(0)
	return v, nil
}

// Update 改节点的值并恢复堆序：比父更优则上浮，否则下沉。
func (h *Heap[T]) Update(n Node, v T) error {
	i, err := h.index(n)
	if err != nil {
		return err
	}
	h.data[i] = v
	h.fix(i)
	return nil
}

func (h *Heap[T]) Get(n Node) (T, error) {
	var zero T
	i, err := h.index(n)
	if err != nil {
		return zero, err
	}
	return h.data[i], nil
}

func (h *Heap[T]) Remove(n Node) error {
	i, err := h.index(n)
	if err != nil {
		return err
	}
	h.removeAt(i)
	return nil
}

func (h *Heap[T]) index(n Node) (int, error) {
	if h == nil || n == 0 {
		return 0, ErrInvalidNode
	}
	i, ok := h.pos[n]
	if !ok {
		return 0, ErrInvalidNode
	}
	return i, nil
}

func (h *Heap[T]) fix(i int) {
	if !h.up(i) {
		h.down(i)
	}
}

func (h *Heap[T]) lessAt(i, j int) bool {
	return h.order.Less(h.data[i], h.data[j])
}

func (h *Heap[T]) swap(i, j int) {
	h.data[i], h.data[j] = h.data[j], h.data[i]
	h.node[i], h.node[j] = h.node[j], h.node[i]
	h.pos[h.node[i]] = i
	h.pos[h.node[j]] = j
}

func (h *Heap[T]) up(i int) bool {
	moved := false
	for i > 0 {
		p := (i - 1) / 2
		if !h.lessAt(i, p) {
			break
		}
		h.swap(i, p)
		i = p
		moved = true
	}
	return moved
}

func (h *Heap[T]) down(i int) {
	n := len(h.data)
	for {
		l := 2*i + 1
		if l >= n {
			return
		}
		best := l
		if r := l + 1; r < n && h.lessAt(r, l) {
			best = r
		}
		if !h.lessAt(best, i) {
			return
		}
		h.swap(i, best)
		i = best
	}
}

func (h *Heap[T]) removeAt(i int) {
	last := len(h.data) - 1
	n := h.node[i]
	h.swap(i, last)
	h.data = h.data[:last]
	h.node = h.node[:last]
	delete(h.pos, n)
	if i < last {
		h.fix(i)
	}
}
