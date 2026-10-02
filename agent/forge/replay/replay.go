package replay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"tunnelforge/agent/forge/capture"
	"tunnelforge/agent/forge/util"
)

const maxCaptureBytes = 1024 * 1024

type ReplayRequest struct {
	Subdomain string `json:"subdomain"`
	RequestID string `json:"request_id"`
	Capture   bool   `json:"capture"`
}

type Replayer struct {
	httpClient *http.Client
}

func NewReplayer(hc *http.Client) *Replayer {
	return &Replayer{
		httpClient: hc,
	}
}

func (r *Replayer) Replay(localAddr string, entry *capture.RequestEntry) (*capture.RequestEntry, error) {
	req, err := r.constructRequest(localAddr, entry)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	res, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	duration := time.Since(start).Milliseconds()

	respCapture := &util.BoundedBuffer{Limit: maxCaptureBytes}
	if _, err := io.Copy(respCapture, res.Body); err != nil {
		return nil, err
	}

	truncated := entry.Truncated || respCapture.TotalRead > maxCaptureBytes

	reqEntry := &capture.RequestEntry{
		ID:              util.UUID(),
		Subdomain:       entry.Subdomain,
		Timestamp:       time.Now(),
		Method:          entry.Method,
		URL:             entry.URL,
		RequestHeaders:  entry.RequestHeaders.Clone(),
		RequestBody:     entry.RequestBody,
		ResponseStatus:  res.StatusCode,
		ResponseHeaders: res.Header.Clone(),
		ResponseBody:    respCapture.Read(),
		DurationMS:      duration,
		Truncated:       truncated,
		Replayed:        true,
	}

	return reqEntry, nil
}

func (r *Replayer) constructRequest(localAddr string, entry *capture.RequestEntry) (*http.Request, error) {
	if !strings.HasPrefix(localAddr, "http://") && !strings.HasPrefix(localAddr, "https://") {
		localAddr = "http://" + localAddr
	}
	urlPath := entry.URL
	if !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	reqUrl := fmt.Sprintf("%s%s", strings.TrimSuffix(localAddr, "/"), urlPath)

	req, err := http.NewRequest(entry.Method, reqUrl, bytes.NewBuffer(entry.RequestBody))
	if err != nil {
		return nil, err
	}

	req.Header = entry.RequestHeaders.Clone()
	req.Header.Del("Connection")
	req.Header.Del("TE")
	req.Header.Del("Transfer-Encoding")
	req.Header.Del("Upgrade")
	req.Header.Del("Keep-Alive")
	req.Header.Del("Proxy-Connection")
	req.Header.Del("Upgrade-Insecure-Requests")
	req.Header.Del("X-Forwarded-For")
	req.Header.Del("X-Forwarded-Proto")
	req.Header.Del("X-Forwarded-Host")

	return req, nil
}
