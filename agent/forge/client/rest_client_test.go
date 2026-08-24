package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"tunnelforge/agent/forge/client"
)

type TestResource struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

func TestRESTClientMultiInstance(t *testing.T) {
	// Server 1 handles Instance 1 requests
	ts1Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(TestResource{ID: "1", Name: "Server1", Value: "GetSuccess"})
		case http.MethodPost:
			var req TestResource
			_ = json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(TestResource{ID: req.ID, Name: "Server1-Created", Value: req.Value})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	ts1Server := httptest.NewServer(ts1Handler)
	defer ts1Server.Close()

	// Server 2 handles Instance 2 requests
	ts2Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPut:
			var req TestResource
			_ = json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(TestResource{ID: req.ID, Name: "Server2-Updated", Value: req.Value})
		case http.MethodPatch:
			var req TestResource
			_ = json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(TestResource{ID: req.ID, Name: "Server2-Patched", Value: req.Value})
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	ts2Server := httptest.NewServer(ts2Handler)
	defer ts2Server.Close()

	// Instantiate multi-instance RESTClients
	client1 := client.NewRESTClient(nil, ts1Server.URL).WithToken("token-1")
	client2 := client.NewRESTClient(nil, ts2Server.URL).WithToken("token-2")

	ctx := context.Background()

	// Test client1 GET
	var res1 TestResource
	err := client1.Get(ctx, "/items/1", &res1)
	if err != nil {
		t.Fatalf("client1 GET error: %v", err)
	}
	if res1.Name != "Server1" || res1.Value != "GetSuccess" {
		t.Errorf("Unexpected result from client1 GET: %+v", res1)
	}

	// Test client1 POST
	var resPost TestResource
	err = client1.Post(ctx, "/items", TestResource{ID: "100", Value: "PostVal"}, &resPost)
	if err != nil {
		t.Fatalf("client1 POST error: %v", err)
	}
	if resPost.Name != "Server1-Created" || resPost.Value != "PostVal" {
		t.Errorf("Unexpected result from client1 POST: %+v", resPost)
	}

	// Test client2 PUT
	var resPut TestResource
	err = client2.Put(ctx, "/items/2", TestResource{ID: "200", Value: "PutVal"}, &resPut)
	if err != nil {
		t.Fatalf("client2 PUT error: %v", err)
	}
	if resPut.Name != "Server2-Updated" || resPut.Value != "PutVal" {
		t.Errorf("Unexpected result from client2 PUT: %+v", resPut)
	}

	// Test client2 PATCH
	var resPatch TestResource
	err = client2.Patch(ctx, "/items/2", TestResource{ID: "200", Value: "PatchVal"}, &resPatch)
	if err != nil {
		t.Fatalf("client2 PATCH error: %v", err)
	}
	if resPatch.Name != "Server2-Patched" || resPatch.Value != "PatchVal" {
		t.Errorf("Unexpected result from client2 PATCH: %+v", resPatch)
	}

	// Test client2 DELETE
	err = client2.Delete(ctx, "/items/2", nil)
	if err != nil {
		t.Fatalf("client2 DELETE error: %v", err)
	}
}

func TestRESTClientHeadersAndAuth(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		customHeader := r.Header.Get("X-Custom-Header")
		if auth != "Bearer secret-token" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		if customHeader != "custom-value" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"missing header"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"status":"ok"}`)
	}))
	defer ts.Close()

	c := client.NewRESTClient(nil, ts.URL).WithToken("secret-token")
	ctx := context.Background()

	opts := client.RequestOpts{
		Method:  http.MethodGet,
		URL:     "/test",
		Headers: map[string]string{"X-Custom-Header": "custom-value"},
	}

	var res map[string]string
	err := c.PerformCustomRequest(ctx, opts, &res)
	if err != nil {
		t.Fatalf("PerformCustomRequest failed: %v", err)
	}
	if res["status"] != "ok" {
		t.Errorf("Expected status ok, got %v", res)
	}
}
