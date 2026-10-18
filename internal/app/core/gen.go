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
