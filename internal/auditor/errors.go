package auditor

import "errors"

var (
	ErrNotInitialized   = errors.New(".regent/ not initialized, run 'rgt-gsd init' first")
	ErrHashNotFound     = errors.New("step hash not found")
	ErrSessionNotFound  = errors.New("session not found")
	ErrRGTNotFound      = errors.New("rgt CLI not found, install re_gent first")
)
