package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/handler"
	"github.com/gejiliang/mihomo-cp/internal/middleware"
	"github.com/gejiliang/mihomo-cp/internal/service"
)

const precheckRequest = `{"operation":"native-precheck","child_context":"child-1","peers":["192.0.2.1"],"budget_ms":1000}`
const precheckSecret = "FAKE_SECRET_NEVER_EXPOSE"

func precheckUpstream(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	calls := new(int)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		if r.Header.Get("Authorization") != "Bearer "+precheckSecret {
			t.Error("existing client credential not used")
		}
		if r.URL.Path == "/rules" {
			io.WriteString(w, `{"rules":[{"payload":"`+precheckSecret+`","proxy":"DIRECT"}]}`)
		} else if r.URL.Path == "/connections" {
			io.WriteString(w, `{"connections":[]}`)
		} else {
			t.Error("unexpected controller path")
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(upstream.Close)
	return upstream, calls
}

func checkPrecheckResponse(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d: %s", rec.Code, status, rec.Body.String())
	}
	if rec.Body.Len() > 16<<10 || strings.Contains(rec.Body.String(), precheckSecret) {
		t.Fatal("unsafe response")
	}
	if !json.Valid(rec.Body.Bytes()) {
		t.Fatal("invalid response JSON")
	}
}

func TestNativePrecheckHandlerStrictInput(t *testing.T) {
	upstream, calls := precheckUpstream(t)
	h := handler.NewNativePrecheckHandler(service.NewNativePrecheckService(service.NewMihomoClient(upstream.URL, precheckSecret)))
	cases := []string{
		`{}`, `null`, `[]`, precheckRequest + ` {}`, precheckRequest + ` garbage`,
		strings.Replace(precheckRequest, `"operation":`, `"operation":"native-precheck","operation":`, 1),
		strings.Replace(precheckRequest, `"operation"`, `"Operation"`, 1),
		strings.Replace(precheckRequest, `"child_context":"child-1"`, `"child_context":null`, 1),
		strings.Replace(precheckRequest, `"budget_ms":1000`, `"budget_ms":0`, 1),
		strings.Replace(precheckRequest, `"budget_ms":1000`, `"budget_ms":1.5`, 1),
		strings.Replace(precheckRequest, `"192.0.2.1"`, `"192.0.2.1","192.0.2.1"`, 1),
		strings.Replace(precheckRequest, `"192.0.2.1"`, `"host.example"`, 1),
		strings.Replace(precheckRequest, `"child-1"`, `"https://example.invalid/secret"`, 1),
		strings.Repeat(" ", 16<<10) + precheckRequest,
	}
	for _, field := range []string{"target", "context", "controller", "path", "url", "env", "shell", "trusted_source", "source", "evidence", "qualified"} {
		cases = append(cases, strings.TrimSuffix(precheckRequest, "}")+`,"`+field+`":"`+precheckSecret+`"}`)
	}
	for i, body := range cases {
		req := httptest.NewRequest("POST", "/api/runtime/native-precheck", strings.NewReader(body))
		// Unknown-length bodies also hit the reader limit.
		req.ContentLength = -1
		rec := httptest.NewRecorder()
		h.Check(rec, req)
		checkPrecheckResponse(t, rec, 400)
		if *calls != 0 {
			t.Fatalf("invalid input %d reached getter", i)
		}
	}
	req := httptest.NewRequest("POST", "/api/runtime/native-precheck?target=x", strings.NewReader(precheckRequest))
	rec := httptest.NewRecorder()
	h.Check(rec, req)
	checkPrecheckResponse(t, rec, 400)
	if *calls != 0 {
		t.Fatal("query selected target")
	}
}

func TestNativePrecheckHandlerAccessAuth(t *testing.T) {
	upstream, calls := precheckUpstream(t)
	h := handler.NewNativePrecheckHandler(service.NewNativePrecheckService(service.NewMihomoClient(upstream.URL, precheckSecret)))
	auth := service.NewAuthService("synthetic-jwt-key", 1, 1)
	refresh, _ := auth.GenerateRefreshToken("fake", "readonly")
	access, _ := auth.GenerateAccessToken("fake", "readonly")
	protected := middleware.Auth(auth)(http.HandlerFunc(h.Check))
	for _, token := range []string{"", "invalid", refresh, access} {
		req := httptest.NewRequest("POST", "/api/runtime/native-precheck", strings.NewReader(precheckRequest))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)
		if token == access {
			checkPrecheckResponse(t, rec, 200)
			if !strings.Contains(rec.Body.String(), `"status":"NOT_BOUND"`) {
				t.Fatal("false qualification")
			}
		} else {
			checkPrecheckResponse(t, rec, 401)
			if *calls != 0 {
				t.Fatal("unauthorized getter call")
			}
		}
	}
	if *calls != 2 {
		t.Fatal("wrong fixed read count")
	}
}

// This is a source binding check, not a claim of a running server adoption.
func TestNativePrecheckServerNormalWiring(t *testing.T) {
	data, err := os.ReadFile("../server/server.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, binding := range []string{
		"handler.NewNativePrecheckHandler(service.NewNativePrecheckService(mihomoClient))",
		"mihomoClient := service.NewMihomoClient(mihomoBaseURL, mihomoSecret)",
		"auth := middleware.Auth(s.authSvc)",
		`s.mux.Handle("POST /api/runtime/native-precheck", protected(s.precheckHandler.Check))`,
	} {
		if !strings.Contains(source, binding) {
			t.Fatalf("missing normal binding %s", binding)
		}
	}
}

func TestNativePrecheckHandlerCancellationAndBudget(t *testing.T) {
	h := handler.NewNativePrecheckHandler(service.NewNativePrecheckService(nil))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("POST", "/api/runtime/native-precheck", strings.NewReader(precheckRequest)).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.Check(rec, req)
	checkPrecheckResponse(t, rec, 504)
	// Reading/parsing time counts against the supplied remaining budget.
	body := &precheckDelayedBody{Reader: strings.NewReader(strings.Replace(precheckRequest, `"budget_ms":1000`, `"budget_ms":1`, 1))}
	req = httptest.NewRequest("POST", "/api/runtime/native-precheck", nil)
	req.Body = body
	rec = httptest.NewRecorder()
	h.Check(rec, req)
	checkPrecheckResponse(t, rec, 504)
}

type precheckDelayedBody struct{ io.Reader }

func (b *precheckDelayedBody) Read(p []byte) (int, error) {
	time.Sleep(5 * time.Millisecond)
	return b.Reader.Read(p)
}
func (*precheckDelayedBody) Close() error { return nil }

func TestNativePrecheckHandlerClosedUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500); io.WriteString(w, precheckSecret) }))
	defer upstream.Close()
	h := handler.NewNativePrecheckHandler(service.NewNativePrecheckService(service.NewMihomoClient(upstream.URL, precheckSecret)))
	rec := httptest.NewRecorder()
	h.Check(rec, httptest.NewRequest("POST", "/api/runtime/native-precheck", strings.NewReader(precheckRequest)))
	checkPrecheckResponse(t, rec, 502)
	if rec.Body.String() != `{"error":{"code":"source_unavailable","message":"native precheck fragments unavailable"}}`+"\n" {
		t.Fatal("error not closed")
	}
}

// A real HTTP body read must stop at the earlier inbound deadline, even when
// the sender never finishes its body. Only a controlled loopback pair is used.
func TestNativePrecheckHandlerSlowInboundBody(t *testing.T) {
	h := handler.NewNativePrecheckHandler(service.NewNativePrecheckService(nil))
	entered := make(chan struct{})
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 40*time.Millisecond)
		defer cancel()
		close(entered)
		h.Check(w, r.WithContext(ctx))
		close(done)
	}))
	defer srv.Close()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	req, err := http.NewRequest("POST", srv.URL, reader)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Second}
	response := make(chan struct{})
	go func() {
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		close(response)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler not entered")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("slow inbound body escaped deadline")
	}
	writer.Close()
	select {
	case <-response:
	case <-time.After(time.Second):
		t.Fatal("loopback client did not finish")
	}
}
