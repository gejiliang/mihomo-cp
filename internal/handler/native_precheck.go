package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/service"
)

const nativePrecheckBodyLimit = 16 << 10

type NativePrecheckHandler struct {
	svc *service.NativePrecheckService
}

func NewNativePrecheckHandler(svc *service.NativePrecheckService) *NativePrecheckHandler {
	return &NativePrecheckHandler{svc: svc}
}

// Check is a read-only POST. Auth is applied by the existing server middleware.
func (h *NativePrecheckHandler) Check(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	ctx, cancel := context.WithDeadline(r.Context(), started.Add(service.NativePrecheckLimit))
	defer cancel()
	deadline, _ := ctx.Deadline()
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(deadline)
	defer controller.SetReadDeadline(time.Time{})
	_ = controller.SetWriteDeadline(deadline)
	defer controller.SetWriteDeadline(time.Time{})
	// Flush while the deadline is still active, before net/http buffers escape
	// this handler. Unsupported controllers (e.g. recorders) remain testable.
	defer controller.Flush()
	// Cancellation closes an in-flight body read as well as controller requests.
	stop := context.AfterFunc(ctx, func() { _ = r.Body.Close() })
	defer stop()
	defer r.Body.Close()
	bad := func() { Error(w, 400, "invalid_request", "invalid native precheck request") }
	if r.URL.RawQuery != "" || r.ContentLength > nativePrecheckBodyLimit {
		bad()
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, nativePrecheckBodyLimit+1))
	if nativePrecheckExpired(ctx) {
		Error(w, 504, "deadline_exceeded", "native precheck deadline exceeded")
		return
	}
	if err != nil || len(body) > nativePrecheckBodyLimit {
		bad()
		return
	}
	// Decode exact field names explicitly: encoding/json's usual struct decoder
	// accepts case-insensitive keys and repeated fields, which this API forbids.
	dec := json.NewDecoder(bytes.NewReader(body))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		bad()
		return
	}
	var in service.NativePrecheckRequest
	seen := make(map[string]bool)
	for dec.More() {
		token, err = dec.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			bad()
			return
		}
		seen[key] = true
		var raw json.RawMessage
		if dec.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			bad()
			return
		}
		switch key {
		case "operation":
			err = json.Unmarshal(raw, &in.Operation)
		case "child_context":
			err = json.Unmarshal(raw, &in.ChildContext)
		case "peers":
			err = json.Unmarshal(raw, &in.Peers)
		case "budget_ms":
			err = json.Unmarshal(raw, &in.BudgetMS)
		default:
			bad()
			return
		}
		if err != nil {
			bad()
			return
		}
	}
	if token, err = dec.Token(); err != nil || token != json.Delim('}') {
		bad()
		return
	}
	if dec.Decode(new(any)) != io.EOF || len(seen) != 4 || !in.Valid() {
		bad()
		return
	}
	// The caller's remaining budget includes the time already spent reading and
	// parsing the request; it is never restarted for each controller response.
	remaining := time.Duration(in.BudgetMS)*time.Millisecond - time.Since(started)
	if remaining <= 0 {
		Error(w, 504, "deadline_exceeded", "native precheck deadline exceeded")
		return
	}
	budgetCtx, budgetCancel := context.WithDeadline(ctx, started.Add(time.Duration(in.BudgetMS)*time.Millisecond))
	defer budgetCancel()
	outputDeadline, _ := budgetCtx.Deadline()
	_ = controller.SetWriteDeadline(outputDeadline)
	result, err := h.svc.Check(budgetCtx, in)
	if err != nil {
		if errors.Is(err, service.ErrNativePrecheckDeadline) {
			Error(w, 504, "deadline_exceeded", "native precheck deadline exceeded")
		} else {
			Error(w, 502, "source_unavailable", "native precheck fragments unavailable")
		}
		return
	}
	data, err := json.Marshal(APIResponse{Data: result})
	if err != nil || len(data) > nativePrecheckBodyLimit {
		Error(w, 502, "source_unavailable", "native precheck fragments unavailable")
		return
	}
	if nativePrecheckExpired(budgetCtx) {
		Error(w, 504, "deadline_exceeded", "native precheck deadline exceeded")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func nativePrecheckExpired(ctx context.Context) bool {
	if ctx.Err() != nil {
		return true
	}
	deadline, ok := ctx.Deadline()
	return ok && !time.Now().Before(deadline)
}
