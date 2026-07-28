package errs

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
)

type Detail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type QueueFullError struct {
	Depth int
}

func (e *QueueFullError) Error() string {
	return fmt.Sprintf("query queue for this connection is full (depth: %d)", e.Depth)
}

func Classify(err error, ctx context.Context) *Detail {
	if ctx.Err() == context.DeadlineExceeded {
		return &Detail{
			Code:    "QUERY_TIMEOUT",
			Message: "query exceeded timeout",
			Hint:    "SQL may need optimization or increase timeout",
		}
	}
	if errors.Is(err, driver.ErrBadConn) {
		return &Detail{
			Code:    "CONNECTION_LOST",
			Message: "connection was killed by database server",
			Hint:    "please reconnect via /connect",
		}
	}
	return &Detail{
		Code:    "DB_ERROR",
		Message: err.Error(),
		Hint:    "check your SQL syntax or database permissions",
	}
}
