package core

import (
	"sync"
	"time"
)

// WaitTimeout waits for wg, at most d. It reports whether wg finished.
// Shutdown uses it so that a stuck background job cannot hang the app.
func WaitTimeout(wg *sync.WaitGroup, d time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-done:
		return true
	case <-t.C:
		return false
	}
}
