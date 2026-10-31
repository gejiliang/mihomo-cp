package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
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

// precheckControllerProbe forwards operations to the real server writer,
// including the Logging wrapper. It never substitutes socket deadline behavior.
type precheckControllerCall struct {
	method   string
	deadline time.Time
	err      error
}

type precheckControllerProbe struct {
	http.ResponseWriter
	calls         []precheckControllerCall
	lateOutput    bool
	writeDeadline time.Time
}

func (p *precheckControllerProbe) SetReadDeadline(deadline time.Time) error {
	err := http.NewResponseController(p.ResponseWriter).SetReadDeadline(deadline)
	p.calls = append(p.calls, precheckControllerCall{"read", deadline, err})
	return err
}
func (p *precheckControllerProbe) SetWriteDeadline(deadline time.Time) error {
	err := http.NewResponseController(p.ResponseWriter).SetWriteDeadline(deadline)
	p.calls = append(p.calls, precheckControllerCall{"write", deadline, err})
	p.writeDeadline = deadline
	return err
}
func (p *precheckControllerProbe) FlushError() error {
	err := http.NewResponseController(p.ResponseWriter).Flush()
	p.calls = append(p.calls, precheckControllerCall{method: "flush", err: err})
	return err
}
func (p *precheckControllerProbe) Write(data []byte) (int, error) {
	if p.lateOutput && !p.writeDeadline.IsZero() {
		// Hold the small response beyond the actual socket deadline. The handler's
		// deferred Flush must enforce the deadline despite net/http buffering.
		time.Sleep(time.Until(p.writeDeadline) + 20*time.Millisecond)
	}
	return p.ResponseWriter.Write(data)
}

func assertPrecheckControllerCalls(t *testing.T, calls []precheckControllerCall, late bool) {
	t.Helper()
	read, write, flush := false, false, false
	var activeWrite time.Time
	for _, call := range calls {
		if call.method == "flush" {
			flush = true
			if activeWrite.IsZero() {
				t.Error("Flush executed after write deadline reset")
			}
			if late {
				var timeout net.Error
				if !errors.As(call.err, &timeout) || !timeout.Timeout() {
					t.Errorf("late Flush did not hit socket timeout: %v", call.err)
				}
			} else if call.err != nil {
				t.Errorf("Flush not forwarded: %v", call.err)
			}
			continue
		}
		if call.err != nil {
			t.Errorf("%s deadline not forwarded: %v", call.method, call.err)
		}
		if call.method == "read" && !call.deadline.IsZero() {
			read = true
		}
		if call.method == "write" {
			activeWrite = call.deadline
			if !call.deadline.IsZero() {
				write = true
			}
		}
	}
	if !read || !write || !flush {
		t.Fatalf("incomplete controller forwarding: %+v", calls)
	}
}

func TestNativePrecheckLoggingDeadlinesAndFlush(t *testing.T) {
	upstream, _ := precheckUpstream(t)
	h := handler.NewNativePrecheckHandler(service.NewNativePrecheckService(service.NewMihomoClient(upstream.URL, precheckSecret)))
	auth := service.NewAuthService("synthetic-logging-jwt", 1, 1)
	token, err := auth.GenerateAccessToken("fake", "readonly")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"normal", "slow-body", "late-output"} {
		t.Run(mode, func(t *testing.T) {
			reports := make(chan []precheckControllerCall, 1)
			entered := make(chan struct{})
			mux := http.NewServeMux()
			mux.Handle("POST /api/runtime/native-precheck", middleware.Auth(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// An earlier incoming deadline keeps the slow-body test short while
				// exercising the handler's unchanged local total-deadline selection.
				ctx, cancel := context.WithTimeout(r.Context(), 100*time.Millisecond)
				defer cancel()
				probe := &precheckControllerProbe{ResponseWriter: w, lateOutput: mode == "late-output"}
				close(entered)
				h.Check(probe, r.WithContext(ctx))
				reports <- probe.calls
			})))
			srv := httptest.NewServer(middleware.Logging(mux))
			defer srv.Close()
			var body io.Reader = strings.NewReader(precheckRequest)
			var pipeWriter *io.PipeWriter
			if mode == "slow-body" {
				reader, writer := io.Pipe()
				defer reader.Close()
				defer writer.Close()
				body = reader
				pipeWriter = writer
			}
			req, err := http.NewRequest("POST", srv.URL+"/api/runtime/native-precheck", body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 2 * time.Second}
			defer client.CloseIdleConnections()
			type response struct {
				status int
				body   string
				err    error
			}
			responses := make(chan response, 1)
			go func() {
				resp, err := client.Do(req)
				if err != nil {
					responses <- response{err: err}
					return
				}
				defer resp.Body.Close()
				data, err := io.ReadAll(io.LimitReader(resp.Body, (16<<10)+1))
				responses <- response{resp.StatusCode, string(data), err}
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("authenticated handler not entered")
			}
			var calls []precheckControllerCall
			select {
			case calls = <-reports:
			case <-time.After(time.Second):
				// Release the synthetic sender on failure so cleanup cannot hang.
				if pipeWriter != nil {
					pipeWriter.Close()
				}
				t.Fatal("Logging chain failed to bound handler completion")
			}
			if pipeWriter != nil {
				pipeWriter.Close()
			}
			var result response
			select {
			case result = <-responses:
			case <-time.After(time.Second):
				t.Fatal("loopback client did not finish")
			}
			assertPrecheckControllerCalls(t, calls, mode != "normal")
			if mode == "normal" {
				if result.err != nil || result.status != 200 || !strings.Contains(result.body, `"status":"NOT_BOUND"`) {
					t.Fatalf("normal Logging response failed: %+v", result)
				}
			} else if strings.Contains(result.body, "NOT_BOUND") {
				t.Fatal("late output escaped expired socket deadline")
			}
			if len(result.body) > 16<<10 || strings.Contains(result.body, precheckSecret) {
				t.Fatal("unsafe Logging response")
			}
		})
	}
}
