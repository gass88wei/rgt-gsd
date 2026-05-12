package executor

import "errors"

var (
	ErrGSDNotFound     = errors.New("gsd-pi CLI not found, run: npm install -g gsd-pi")
	ErrSessionFailed   = errors.New("gsd-2 session failed to start")
	ErrSessionTimeout  = errors.New("gsd-2 session timed out")
	ErrSessionNotFound = errors.New("session not found")
	ErrAlreadyActive   = errors.New("another gsd-2 session is active in this project")
)
