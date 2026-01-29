package core

import (
	"container/heap"
	"sync"
)

// TaskPriority 任务优先级枚举
type TaskPriority int

const (
	PriorityLow TaskPriority = iota
	PriorityNormal
	PriorityHigh
)

// PriorityTask 带优先级的监控任务
type PriorityTask struct {
	Target   *MonitorTarget
	Priority TaskPriority
	Index    int // 用于堆操作
}

// PriorityQueue 优先级队列实现
type PriorityQueue []*PriorityTask

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// 优先级高的排在前面
	return pq[i].Priority > pq[j].Priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	task := x.(*PriorityTask)
	task.Index = n
	*pq = append(*pq, task)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	task := old[n-1]
	old[n-1] = nil  // 防止内存泄漏
	task.Index = -1 // 标记为已弹出
	*pq = old[0 : n-1]
	return task
}

// ConcurrencyLimiter 增强版并发限制器（支持优先级）
type ConcurrencyLimiter struct {
	sem    chan struct{}
	pq     PriorityQueue
	mu     sync.Mutex
	cond   *sync.Cond
	closed bool
}

// NewConcurrencyLimiter 创建带优先级的并发限制器
func NewConcurrencyLimiter(max int) *ConcurrencyLimiter {
	cl := &ConcurrencyLimiter{
		sem:    make(chan struct{}, max),
		closed: false,
	}
	cl.cond = sync.NewCond(&cl.mu)
	heap.Init(&cl.pq)
	return cl
}

// AcquireWithPriority 带优先级获取执行权限
func (cl *ConcurrencyLimiter) AcquireWithPriority(task *PriorityTask) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.closed {
		panic("ConcurrencyLimiter已关闭")
	}

	// 将任务加入优先级队列
	heap.Push(&cl.pq, task)
	cl.cond.Signal()

	// 等待可用槽位
	for len(cl.sem) == cap(cl.sem) {
		cl.cond.Wait()
	}

	// 弹出最高优先级任务执行
	execTask := heap.Pop(&cl.pq).(*PriorityTask)
	cl.sem <- struct{}{}

	// 确保执行的是当前任务（防止优先级抢占）
	if execTask != task {
		// 放回被抢占的任务
		heap.Push(&cl.pq, execTask)
		// 重新等待
		cl.AcquireWithPriority(task)
	}
}

// Acquire 兼容原有方法（默认普通优先级）
func (cl *ConcurrencyLimiter) Acquire() {
	task := &PriorityTask{
		Priority: PriorityNormal,
	}
	cl.AcquireWithPriority(task)
}

// Release 释放并发执行权限
func (cl *ConcurrencyLimiter) Release() {
	<-cl.sem
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.cond.Signal()
}

// Close 关闭限制器（清理资源）
func (cl *ConcurrencyLimiter) Close() {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.closed = true
	close(cl.sem)
	cl.cond.Broadcast()
}
