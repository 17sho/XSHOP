package contract

import "errors"

var (
	// ErrSubmissionNotDispatched is only for adapter checks before any network send.
	ErrSubmissionNotDispatched = errors.New("supplier submission was not dispatched")
	ErrNotFound                = errors.New("procurement order not found")
	ErrExists                  = errors.New("procurement order already exists")
	ErrStatusInvalid           = errors.New("procurement order status invalid")
	ErrOrderNotFound           = errors.New("order not found")
	ErrConnectionNotFound      = errors.New("site connection not found")
)
