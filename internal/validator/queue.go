package validator

import "container/heap"

type priorityQueue struct {
	seq int64
	h   taskHeap
}

type pqItem struct {
	task Task
	seq  int64
}

func newPriorityQueue() *priorityQueue {
	pq := &priorityQueue{}
	heap.Init(&pq.h)
	return pq
}

func (pq *priorityQueue) Push(t Task) {
	pq.seq++
	heap.Push(&pq.h, pqItem{task: t, seq: pq.seq})
}

func (pq *priorityQueue) Pop() (Task, bool) {
	if pq.h.Len() == 0 {
		return Task{}, false
	}
	it := heap.Pop(&pq.h).(pqItem)
	return it.task, true
}

type taskHeap []pqItem

func (h taskHeap) Len() int { return len(h) }
func (h taskHeap) Less(i, j int) bool {
	a := h[i]
	b := h[j]
	if a.task.Priority != b.task.Priority {
		return a.task.Priority < b.task.Priority
	}
	return a.seq < b.seq
}
func (h taskHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *taskHeap) Push(x any) {
	*h = append(*h, x.(pqItem))
}

func (h *taskHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	*h = old[:n-1]
	return it
}

