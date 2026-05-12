package recovery

import "errors"

var (
	ErrUnrecoverable = errors.New("unrecoverable failure, human intervention required")
	ErrRewindFailed  = errors.New("failed to rewind workspace")
)
