package ffplayer

import "sync"

// ring is the decoded audio between the audio reader goroutine and the main
// thread: interleaved stereo float32 frames, bounded. write blocks while it
// is full; read never blocks.
type ring struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    []float32 // 2 floats per frame
	start  int       // first frame
	n      int       // frames held
	closed bool
}

func newRing(frames int) *ring {
	r := &ring{buf: make([]float32, 2*max(frames, 1))}
	r.cond = sync.NewCond(&r.mu)
	return r
}

func (r *ring) capFrames() int { return len(r.buf) / 2 }

// write appends src (whole frames), blocking while the ring is full. It
// reports false once the ring is closed.
func (r *ring) write(src []float32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.capFrames()
	for len(src) >= 2 {
		for r.n == c && !r.closed {
			r.cond.Wait()
		}
		if r.closed {
			return false
		}
		k := min(c-r.n, len(src)/2)
		end := (r.start + r.n) % c
		first := min(k, c-end)
		copy(r.buf[2*end:], src[:2*first])
		copy(r.buf, src[2*first:2*k])
		r.n += k
		src = src[2*k:]
	}
	return !r.closed
}

// read moves up to len(dst)/2 frames into dst and returns how many.
func (r *ring) read(dst []float32) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.capFrames()
	k := min(r.n, len(dst)/2)
	if k == 0 {
		return 0
	}
	first := min(k, c-r.start)
	copy(dst, r.buf[2*r.start:2*(r.start+first)])
	copy(dst[2*first:], r.buf[:2*(k-first)])
	r.start = (r.start + k) % c
	r.n -= k
	r.cond.Broadcast()
	return k
}

// frames is how many frames are held.
func (r *ring) frames() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.n
}

// close wakes a blocked writer; later writes fail.
func (r *ring) close() {
	r.mu.Lock()
	r.closed = true
	r.cond.Broadcast()
	r.mu.Unlock()
}
