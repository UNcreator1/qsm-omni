package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeRouteHealthClassifiesRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
	})
	mux.HandleFunc("/sse/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\n"))
	})
	mux.HandleFunc("/bad/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"missing credentials"}}`, http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	okCfg := Config{NineRouterURL: server.URL + "/v1", NineRouterKey: "key"}
	results := ProbeRouteHealth(context.Background(), okCfg, []string{"a", "a"}, time.Second)
	if len(results) != 1 || !results[0].OK || results[0].ResponseShape != "openai_chat_json" {
		t.Fatalf("unexpected ok result: %#v", results)
	}

	sseCfg := Config{NineRouterURL: server.URL + "/sse", NineRouterKey: "key"}
	sse := probeOneRoute(context.Background(), sseCfg, "a", time.Second)
	if sse.OK || sse.ResponseShape != "sse_stream" {
		t.Fatalf("expected sse failure, got %#v", sse)
	}

	badCfg := Config{NineRouterURL: server.URL + "/bad", NineRouterKey: "key"}
	bad := probeOneRoute(context.Background(), badCfg, "a", time.Second)
	if bad.OK || bad.ResponseShape != "http_error" {
		t.Fatalf("expected http failure, got %#v", bad)
	}
}
