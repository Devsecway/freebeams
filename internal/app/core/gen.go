package core

// Generation drops stale async results. Each request takes a new ticket with
// Next; when its result arrives on the main thread, Current reports whether it
// is still the latest request. A search typed "a", "am", "ama" fires three
// queries; only the result of "ama" is applied, whatever order they finish in.
//
// Generation is not safe for concurrent use: call it on the main thread only
// (goroutines carry their ticket by value).
type Generation struct {
	n uint64
}

// Next starts a new request and returns its ticket.
func (g *Generation) Next() uint64 {
	g.n++
	return g.n
}

// Current reports whether ticket belongs to the latest request.
func (g *Generation) Current(ticket uint64) bool { return ticket != 0 && ticket == g.n }

// Latest returns the latest ticket (0 before the first request).
func (g *Generation) Latest() uint64 { return g.n }

// Sequencer applies async results in request order. Each request takes a
// ticket with Next; when its result arrives (in any order), Done holds it
// until every earlier ticket is done too, then runs the ready results in
// ticket order. An older request therefore never overrides a newer one.
//
// A result can still tell whether it was overtaken (a newer request exists)
// with Superseded, to drop pure display work such as a preview.
//
// Sequencer is not safe for concurrent use: call it on the main thread only.
type Sequencer struct {
	next    uint64            // last ticket handed out
	applied uint64            // every ticket <= applied has run
	ready   map[uint64]func() // finished, waiting for earlier tickets
}

// Next starts a new request and returns its ticket.
func (s *Sequencer) Next() uint64 {
	s.next++
	return s.next
}

// Superseded reports whether a newer request than ticket has been started.
func (s *Sequencer) Superseded(ticket uint64) bool { return ticket < s.next }

// Done records ticket's result and runs, in ticket order, every result whose
// predecessors have all run. Unknown or repeated tickets are ignored. Every
// ticket must eventually be Done (with a no-op on failure), or later results
// wait forever.
func (s *Sequencer) Done(ticket uint64, apply func()) {
	if ticket <= s.applied || ticket > s.next {
		return
	}
	if _, dup := s.ready[ticket]; dup {
		return
	}
	if s.ready == nil {
		s.ready = map[uint64]func(){}
	}
	if apply == nil {
		apply = func() {}
	}
	s.ready[ticket] = apply
	for {
		fn, ok := s.ready[s.applied+1]
		if !ok {
			return
		}
		delete(s.ready, s.applied+1)
		s.applied++
		fn()
	}
}

// Waiting returns how many finished results wait for an earlier one.
func (s *Sequencer) Waiting() int { return len(s.ready) }
