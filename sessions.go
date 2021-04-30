package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

type sessions struct {
	list            map[string]time.Time
	lock            *sync.RWMutex
	lifeTime        time.Duration
	contextCancelFn context.CancelFunc
	wg              sync.WaitGroup
}

func sessionsNew(ctx context.Context, lifeTime time.Duration) *sessions {
	var s sessions
	s.list = make(map[string]time.Time)
	s.lifeTime = lifeTime
	s.lock = new(sync.RWMutex)

	var gcContext context.Context
	gcContext, s.contextCancelFn = context.WithCancel(ctx)

	// run sessions GC
	s.wg.Add(1)
	go s.gc(gcContext, lifeTime)

	return &s
}

// session garbage collector
func (s *sessions) gc(ctx context.Context, lifeTime time.Duration) {
	log.Println("Session gc worker start")
	defer s.wg.Done()
	defer log.Println("Session gc worker stop")
	// we don't wanna lock session list when session expired and client needs relogin
	timer := time.NewTicker(lifeTime + (lifeTime / 2))
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			st := time.Now()
			s.lock.Lock()
			delCount := 0
			for session, expire := range s.list {
				if time.Now().After(expire) {
					delete(s.list, session)
					delCount++
				}
			}
			activeCount := len(s.list)
			s.lock.Unlock()

			log.Printf("Session gc worker delete %d expired session. Active sessions %d. GC duration %v.\n", delCount, activeCount, time.Now().Sub(st))
		}
	}
}

// create new session
var errSessionColision = errors.New("Session collision detected")

func (s *sessions) new() (string, time.Time, error) {
	uuid, err := uuid.NewRandom()
	if err != nil {
		return "", time.Time{}, err
	}

	sesstr := uuid.String()
	s.lock.RLock()
	defer s.lock.RUnlock()
	if _, ok := s.list[sesstr]; ok {
		return "", time.Time{}, errSessionColision
	}

	expire := time.Now().Add(s.lifeTime)
	s.list[sesstr] = expire
	return sesstr, expire, nil
}

// check session key
func (s *sessions) check(session string) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()
	expire, ok := s.list[session]
	if ok && time.Now().After(expire) {
		return false
	}
	return ok
}

// delete session
func (s *sessions) logout(session string) {
	if s.check(session) {
		s.lock.Lock()
		defer s.lock.Unlock()
		delete(s.list, session)
	}
}

// stop gc worker
func (s *sessions) gcStop() {
	s.contextCancelFn()
	s.wg.Wait()
}
