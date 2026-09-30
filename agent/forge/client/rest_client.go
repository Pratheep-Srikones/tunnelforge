package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var HTTPErrorCodes = map[int]string{
	400: "Bad request. Provided parameters are not in correct format.",
	401: "Unauthorized access.",
	403: "Forbidden request. \nYou do not have permission to perform this action.",
	404: "Resource not found.",
	405: "The requested method is not provided for the resource type.",
	409: "A resource with the same identifier already exists.",
	500: "Internal server error.",
}

func getErrorMessage(statusCode int) string {
	message, ok := HTTPErrorCodes[statusCode]
	if !ok {
		return "Unknown error."
	}
	return message
}

type RequestOpts struct {
	Method  string
	URL     string
	Payload any
	Headers map[string]string
	Token   string
}

type RESTClient struct {
	ServerURL  string
	Token      string
	HTTPClient *http.Client
}

func NewRESTClient(hc *http.Client, serverURL string) *RESTClient {
	if hc == nil {
		hc = GetHTTPClient(false, 10*time.Second)
	}
	if serverURL != "" && !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		serverURL = "http://" + serverURL
	}
	return &RESTClient{
		ServerURL:  serverURL,
		HTTPClient: hc,
	}
}

// WithToken sets the default Bearer token for the RESTClient instance.
func (c *RESTClient) WithToken(token string) *RESTClient {
	c.Token = token
	return c
}

// Get performs an HTTP GET request to the specified URL or relative path.
func (c *RESTClient) Get(ctx context.Context, url string, result any) error {
	return c.PerformRequest(ctx, http.MethodGet, url, nil, result)
}

// Post performs an HTTP POST request with a payload to the specified URL or relative path.
func (c *RESTClient) Post(ctx context.Context, url string, payload any, result any) error {
	return c.PerformRequest(ctx, http.MethodPost, url, payload, result)
}

// Put performs an HTTP PUT request with a payload to the specified URL or relative path.
func (c *RESTClient) Put(ctx context.Context, url string, payload any, result any) error {
	return c.PerformRequest(ctx, http.MethodPut, url, payload, result)
}

// Patch performs an HTTP PATCH request with a payload to the specified URL or relative path.
func (c *RESTClient) Patch(ctx context.Context, url string, payload any, result any) error {
	return c.PerformRequest(ctx, http.MethodPatch, url, payload, result)
}

// Delete performs an HTTP DELETE request to the specified URL or relative path.
func (c *RESTClient) Delete(ctx context.Context, url string, result any) error {
	return c.PerformRequest(ctx, http.MethodDelete, url, nil, result)
}

// PerformRequest executes an HTTP request using method, URL/path, payload, and unmarshals result into the provided struct.
func (c *RESTClient) PerformRequest(ctx context.Context, method string, url string, payload any, result any) error {
	opts := RequestOpts{
		Method:  method,
		URL:     url,
		Payload: payload,
	}
	return c.PerformCustomRequest(ctx, opts, result)
}

// PerformCustomRequest executes an HTTP request using custom options (RequestOpts).
func (c *RESTClient) PerformCustomRequest(ctx context.Context, opts RequestOpts, result any) error {
	bodyReader, err := c.preparePayload(opts.Payload)
	if err != nil {
		return err
	}

	req, err := c.buildRequest(ctx, opts, bodyReader)
	if err != nil {
		return err
	}

	return c.executeAndDecode(req, result)
}

func (c *RESTClient) preparePayload(payload any) (io.Reader, error) {
	if payload == nil {
		return nil, nil
	}

	if reader, ok := payload.(io.Reader); ok {
		return reader, nil
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("request encoding: %w", err)
	}

	return bytes.NewReader(jsonBytes), nil
}

func (c *RESTClient) resolveURL(urlStr string) string {
	if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
		return urlStr
	}
	serverURL := c.ServerURL
	if serverURL != "" && !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		serverURL = "http://" + serverURL
	}
	if serverURL == "" {
		return urlStr
	}
	base := strings.TrimSuffix(serverURL, "/")
	path := strings.TrimPrefix(urlStr, "/")
	return base + "/" + path
}

func (c *RESTClient) buildRequest(ctx context.Context, opts RequestOpts, bodyReader io.Reader) (*http.Request, error) {
	targetURL := c.resolveURL(opts.URL)

	token := opts.Token
	if token == "" {
		token = c.Token
	}

	var req *http.Request
	var err error

	if token != "" {
		req, err = http.NewRequestWithContext(ctx, opts.Method, targetURL, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = c.NewAuthorizedRequest(ctx, opts.Method, targetURL, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("creating authorized request: %w", err)
		}
	}

	if opts.Headers != nil {
		for key, value := range opts.Headers {
			req.Header.Set(key, value)
		}
	}

	return req, nil
}

func (c *RESTClient) executeAndDecode(req *http.Request, result any) error {
	resp, err := c.SendRequest(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.ValidateResponse(resp); err != nil {
		return fmt.Errorf("validating response: %w", err)
	}

	if result == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}

	err = json.NewDecoder(resp.Body).Decode(result)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}

func (c *RESTClient) SendRequest(req *http.Request) (*http.Response, error) {
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	return resp, nil
}

func (c *RESTClient) ValidateResponse(resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading response body: %w", err)
		}
		resp.Body = io.NopCloser(bytes.NewBuffer(data))
		return fmt.Errorf("response failed (%d) %s: %s", resp.StatusCode, getErrorMessage(resp.StatusCode), string(data))
	}
	return nil
}

func (c *RESTClient) NewAuthorizedRequest(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	targetURL := c.resolveURL(url)
	req, err := http.NewRequestWithContext(ctx, method, targetURL, body)
	if err != nil {
		return nil, err
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (c *RESTClient) NewRequestWithContext(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	targetURL := c.resolveURL(path)
	req, err := http.NewRequestWithContext(ctx, method, targetURL, body)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func (c *RESTClient) BuildRequestURL(suffix string, query string, additionalComps ...string) string {
	var url strings.Builder
	url.WriteString(c.ServerURL)
	if suffix != "" {
		url.WriteString("/")
		url.WriteString(suffix)
	}
	for _, path := range additionalComps {
		url.WriteString("/")
		url.WriteString(path)
	}
	if query != "" {
		url.WriteString(query)
	}
	return url.String()
}

func GetHTTPClient(insecureSkipVerify bool, timeout time.Duration) *http.Client {
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecureSkipVerify,
			},
		},
	}
	return client
}
