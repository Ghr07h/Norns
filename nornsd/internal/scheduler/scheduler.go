package scheduler

import (
	"context"
	"sync"
)

// Claim atomically claims the oldest queued Runtime for slot. Claimed is
// false when the persistent queue is empty.
type Claim func(ctx context.Context, slot int) (claimed bool, err error)

// Scheduler owns stable, one-based concurrency slots. Notifications are
// coalesced; the database queue remains the source of truth.
type Scheduler struct {
	ctx     context.Context
	cancel  context.CancelFunc
	claim   Claim
	wake    chan struct{}
	release chan int
	done    chan struct{}
	onError func(error)

	mu       sync.RWMutex
	alive    bool
	draining bool
	free     []bool
}

func New(parent context.Context, concurrency int, claim Claim, onError func(error)) *Scheduler {
	ctx, cancel := context.WithCancel(parent)
	free := make([]bool, concurrency+1)
	for i := 1; i <= concurrency; i++ {
		free[i] = true
	}
	return &Scheduler{ctx: ctx, cancel: cancel, claim: claim, wake: make(chan struct{}, 1), release: make(chan int, concurrency), done: make(chan struct{}), onError: onError, free: free}
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.alive {
		s.mu.Unlock()
		return
	}
	s.alive = true
	s.mu.Unlock()
	go s.loop()
	s.Notify()
}

func (s *Scheduler) loop() {
	defer close(s.done)
	for {
		select {
		case <-s.ctx.Done():
			return
		case slot := <-s.release:
			if slot > 0 && slot < len(s.free) {
				s.free[slot] = true
			}
		case <-s.wake:
		}
		for !s.isDraining() {
			slot := s.nextFree()
			if slot == 0 {
				break
			}
			claimed, err := s.claim(s.ctx, slot)
			if err != nil {
				if s.onError != nil {
					s.onError(err)
				}
				break
			}
			if !claimed {
				break
			}
			s.free[slot] = false
		}
	}
}

func (s *Scheduler) nextFree() int {
	for i := 1; i < len(s.free); i++ {
		if s.free[i] {
			return i
		}
	}
	return 0
}
func (s *Scheduler) isDraining() bool { s.mu.RLock(); defer s.mu.RUnlock(); return s.draining }
func (s *Scheduler) Notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *Scheduler) Release(slot int) {
	select {
	case s.release <- slot:
	case <-s.ctx.Done():
	}
	s.Notify()
}
func (s *Scheduler) Drain()        { s.mu.Lock(); s.draining = true; s.mu.Unlock() }
func (s *Scheduler) Healthy() bool { s.mu.RLock(); defer s.mu.RUnlock(); return s.alive && !s.draining }
func (s *Scheduler) Close() {
	s.mu.RLock()
	alive := s.alive
	s.mu.RUnlock()
	s.cancel()
	if alive {
		<-s.done
	}
	s.mu.Lock()
	s.alive = false
	s.mu.Unlock()
}
