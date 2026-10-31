package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const nativeSentinel = "SECRET_SENTINEL_NEVER_PROJECT"

func nativeInput() NativePrecheckRequest {
	return NativePrecheckRequest{"native-precheck", "child-1", []string{"192.0.2.1", "192.0.2.2"}, 1000}
}

// All transports/controllers here are synthetic, with no environment proxy.
func nativeClient(t *testing.T, fn http.HandlerFunc) *MihomoClient {
	t.Helper()
	srv := httptest.NewServer(fn)
	t.Cleanup(srv.Close)
	mc := NewMihomoClient(srv.URL, nativeSentinel)
	mc.client = srv.Client()
	mc.client.Transport = &http.Transport{Proxy: nil}
	return mc
}

func TestNativePrecheckNormalFragmentsNeverQualify(t *testing.T) {
	var paths []string
	mc := nativeClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer "+nativeSentinel {
			t.Error("normal client credential not reused")
		}
		if r.URL.Path == "/rules" {
			io.WriteString(w, `{"rules":[{"type":"`+nativeSentinel+`","payload":"`+nativeSentinel+`","proxy":"DIRECT","metadata":{"secret":"`+nativeSentinel+`"}}]}`)
		} else {
			io.WriteString(w, `{"connections":[],"metadata":{"secret":"`+nativeSentinel+`"}}`)
		}
	})
	for i := 0; i < 2; i++ {
		result, err := NewNativePrecheckService(mc).Check(context.Background(), nativeInput())
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != "NOT_BOUND" || result.Fragments.RuleCount != 1 || result.Fragments.ConnectionCount != 0 {
			t.Fatalf("unexpected projection: %+v", result)
		}
		facts, _ := json.Marshal(result.Facts)
		var values map[string]string
		json.Unmarshal(facts, &values)
		if len(values) != 10 {
			t.Fatal("missing explicit unknown facts")
		}
		for _, v := range values {
			if v != "UNKNOWN" {
				t.Fatal("fragment became evidence")
			}
		}
		data, _ := json.Marshal(result)
		if len(data) > 16<<10 || strings.Contains(string(data), nativeSentinel) || strings.Contains(string(data), "DIRECT") || strings.Contains(string(data), "qualified") {
			t.Fatal("unsafe projection")
		}
	}
	if strings.Join(paths, ",") != "GET /rules,GET /connections,GET /rules,GET /connections" {
		t.Fatalf("unexpected reads %v", paths)
	}
}

func TestNativePrecheckInput(t *testing.T) {
	tests := []NativePrecheckRequest{
		{"other", "ctx", []string{"192.0.2.1"}, 100},
		{"native-precheck", "https://secret", []string{"192.0.2.1"}, 100},
		{"native-precheck", "ctx", nil, 100},
		{"native-precheck", "ctx", []string{"192.0.2.1", "192.0.2.1"}, 100},
		{"native-precheck", "ctx", []string{"::1"}, 100},
		{"native-precheck", "ctx", []string{"192.0.2.1"}, 0},
		{"native-precheck", "ctx", []string{"192.0.2.1"}, 20001},
	}
	for _, in := range tests {
		_, err := NewNativePrecheckService(nil).Check(context.Background(), in)
		if !errors.Is(err, ErrNativePrecheckInput) {
			t.Fatalf("bad input accepted: %v", err)
		}
	}
}

func TestNativePrecheckUpstreamFailuresClosed(t *testing.T) {
	for _, mode := range []string{"400", "500", "redirect", "oversize", "invalid", "tail", "null", "missing", "entry", "partial", "transport"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			mc := nativeClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				switch mode {
				case "400":
					w.WriteHeader(400)
				case "500":
					w.WriteHeader(500)
				case "redirect":
					w.Header().Set("Location", "/stolen")
					w.WriteHeader(302)
				case "oversize":
					io.WriteString(w, strings.Repeat(" ", 64<<10)+nativeSentinel)
				case "invalid":
					io.WriteString(w, nativeSentinel)
				case "tail":
					io.WriteString(w, `{"rules":[]} {}`)
				case "null":
					io.WriteString(w, `{"rules":null}`)
				case "missing":
					io.WriteString(w, `{"draft":{"rules":[]}}`)
				case "entry":
					io.WriteString(w, `{"rules":[null]}`)
				case "partial":
					if calls == 1 {
						io.WriteString(w, `{"rules":[{"payload":"`+nativeSentinel+`"}]}`)
						return
					}
					w.WriteHeader(500)
				}
				io.WriteString(w, nativeSentinel)
			})
			if mode == "transport" {
				mc.client.Transport = nativeRoundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New(nativeSentinel) })
			}
			result, err := NewNativePrecheckService(mc).Check(context.Background(), nativeInput())
			if err != ErrNativePrecheckSource || result != (NativePrecheckResult{}) || strings.Contains(err.Error(), nativeSentinel) {
				t.Fatalf("not closed: %+v %v", result, err)
			}
			expected := 1
			if mode == "partial" {
				expected = 2
			}
			if mode == "transport" {
				expected = 0
			}
			if calls != expected {
				t.Fatalf("redirect/retry/fallback: %d calls", calls)
			}
		})
	}
}

type nativeRoundTrip func(*http.Request) (*http.Response, error)

func (f nativeRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNativePrecheckSharedDeadlineAndLateData(t *testing.T) {
	mc := NewMihomoClient("http://synthetic.invalid", nativeSentinel)
	var first time.Time
	calls := 0
	mc.client.Transport = nativeRoundTrip(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Fatal("no read deadline")
		}
		if calls == 0 {
			first = deadline
		} else if !deadline.Equal(first) {
			t.Error("budget reopened")
		}
		calls++
		time.Sleep(15 * time.Millisecond)
		field := "rules"
		if calls == 2 {
			field = "connections"
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"` + field + `":[]}`)), Header: make(http.Header), Request: r}, nil
	})
	in := nativeInput()
	in.BudgetMS = 25
	_, err := NewNativePrecheckService(mc).Check(context.Background(), in)
	if err != ErrNativePrecheckDeadline || calls != 2 {
		t.Fatalf("late legal data accepted: %v calls %d", err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewNativePrecheckService(mc).Check(ctx, nativeInput())
	if err != ErrNativePrecheckDeadline || calls != 2 {
		t.Fatal("canceled request performed reads")
	}
	calls = 0 // A new invocation has its own earlier parent deadline.
	ctx, cancel = context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err = NewNativePrecheckService(mc).Check(ctx, nativeInput())
	if err != ErrNativePrecheckDeadline {
		t.Fatal("earlier parent deadline ignored")
	}
}

func TestNativePrecheckSlowAndInfiniteBody(t *testing.T) {
	for _, infinite := range []bool{false, true} {
		t.Run(map[bool]string{false: "slow", true: "infinite"}[infinite], func(t *testing.T) {
			mc := nativeClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
				if !infinite {
					<-r.Context().Done()
					return
				}
				chunk := strings.Repeat(" ", 4096)
				for {
					if _, err := io.WriteString(w, chunk); err != nil {
						return
					}
					w.(http.Flusher).Flush()
					select {
					case <-r.Context().Done():
						return
					default:
					}
				}
			})
			in := nativeInput()
			in.BudgetMS = 50
			start := time.Now()
			_, err := NewNativePrecheckService(mc).Check(context.Background(), in)
			if err == nil || time.Since(start) > time.Second {
				t.Fatalf("unbounded body: %v", err)
			}
		})
	}
}

func TestNativePrecheckUnboundContextAndPeers(t *testing.T) {
	mc := nativeClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rules" {
			io.WriteString(w, `{"rules":[{"proxy":"DIRECT"}],"dns":{"fake-ip":false},"loaded":true}`)
		} else {
			io.WriteString(w, `{"connections":[]}`)
		}
	})
	for _, contextRef := range []string{"wrong-context", "expired-context"} {
		in := nativeInput()
		in.ChildContext = contextRef
		in.Peers = []string{"198.51.100.99"}
		result, err := NewNativePrecheckService(mc).Check(context.Background(), in)
		if err != nil || result.Status != "NOT_BOUND" || result.Facts.TargetBinding != "UNKNOWN" || result.Facts.EffectivePolicy != "UNKNOWN" {
			t.Fatal("unbound claims became evidence")
		}
	}
}

func TestNativePrecheckExactControllerLimit(t *testing.T) {
	for _, size := range []int{64 << 10, (64 << 10) + 1} {
		mc := nativeClient(t, func(w http.ResponseWriter, r *http.Request) {
			body := `{"rules":[],"connections":[]}`
			io.WriteString(w, body+strings.Repeat(" ", size-len(body)))
		})
		result, err := NewNativePrecheckService(mc).Check(context.Background(), nativeInput())
		if size == 64<<10 {
			if err != nil || result.Status != "NOT_BOUND" {
				t.Fatal("exact limit rejected")
			}
		} else if err != ErrNativePrecheckSource {
			t.Fatal("limit+1 accepted")
		}
	}
}
