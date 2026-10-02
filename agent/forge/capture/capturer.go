package capture

import (
	"net/http"
	"time"
)

type Capturer interface {
	Push(subdomain string, req *RequestEntry) error
	List(subdomain string, limit int) ([]*RequestEntry, error)
	Get(subdomain string, id string) (*RequestEntry, error)
	Remove(subdomain string, id string) error
	Clear(subdomain string) error
}

type RequestEntry struct {
	ID              string      `json:"id"`
	Subdomain       string      `json:"subdomain"`
	Timestamp       time.Time   `json:"timestamp"`
	Method          string      `json:"method"`
	URL             string      `json:"url"`
	RequestHeaders  http.Header `json:"request_headers"`
	RequestBody     []byte      `json:"request_body"`
	ResponseStatus  int         `json:"response_status"`
	ResponseHeaders http.Header `json:"response_headers"`
	ResponseBody    []byte      `json:"response_body"`
	DurationMS      int64       `json:"duration_ms"`
	Truncated       bool        `json:"truncated"`
	Err             string      `json:"err,omitempty"`
	Replayed        bool        `json:"replayed,omitempty"`
}
