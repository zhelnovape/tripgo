package push

import (
	"fmt"
	"net/http"
)

type StatusError struct {
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("push service responded %d %s", e.StatusCode, http.StatusText(e.StatusCode))
}

func (e *StatusError) Temporary() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= http.StatusInternalServerError
}

type TransportError struct {
	Err error
}

func (e *TransportError) Error() string {
	return "push service transport: " + e.Err.Error()
}

func (e *TransportError) Unwrap() error {
	return e.Err
}
