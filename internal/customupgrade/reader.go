package customupgrade

import (
	"context"
	"io"
)

// contextReader bounds empty reads and checks cancellation at every read. An
// arbitrary blocked io.Reader cannot be safely interrupted; callers must supply
// a context-aware reader or enforce I/O deadlines. No goroutines are leaked.
type contextReader struct {
	ctx   context.Context
	r     io.Reader
	empty int
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if canceled := r.ctx.Err(); canceled != nil {
		return n, canceled
	}
	if n == 0 && err == nil {
		r.empty++
		if r.empty >= 100 {
			return 0, io.ErrNoProgress
		}
	} else {
		r.empty = 0
	}
	return n, err
}
