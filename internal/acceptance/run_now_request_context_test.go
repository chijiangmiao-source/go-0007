package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"leo-debris-orbit-loop/internal/api"
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/orbit"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type requestContextKey struct{}

type requestContextEngine struct {
	started  chan struct{}
	observed chan bool
	once     sync.Once
}

func newRequestContextEngine() *requestContextEngine {
	return &requestContextEngine{started: make(chan struct{}), observed: make(chan bool, 1)}
}

func (e *requestContextEngine) Version() string { return "request-context-engine-v1" }

func (e *requestContextEngine) Compute(ctx context.Context, _ orbit.InputSnapshot) (orbit.EngineResult, error) {
	attached := ctx.Value(requestContextKey{}) == "attached"
	e.observed <- attached
	e.once.Do(func() { close(e.started) })
	if !attached {
		return orbit.EngineResult{}, errors.New("HTTP request context was detached")
	}
	<-ctx.Done()
	return orbit.EngineResult{}, ctx.Err()
}

func TestModel_RunNowSolveInheritsRequestCancellation(t *testing.T) {
	tests := []struct {
		name       string
		newContext func() (context.Context, context.CancelFunc)
		cancelNow  bool
		wantStatus domain.SolveJobStatus
		wantClass  string
		wantErr    error
	}{
		{
			name: "client cancellation",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.WithValue(context.Background(), requestContextKey{}, "attached"))
			},
			cancelNow:  true,
			wantStatus: domain.JobCanceled,
			wantClass:  string(domain.CodeCanceled),
			wantErr:    context.Canceled,
		},
		{
			name: "request deadline",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.WithValue(context.Background(), requestContextKey{}, "attached"), time.Now().Add(-time.Second))
			},
			wantStatus: domain.JobTimedOut,
			wantClass:  string(domain.CodeTimeout),
			wantErr:    context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := newRequestContextEngine()
			app := api.NewApp(t.TempDir()+"/state.json", engine)
			if err := app.Recovery.Recover(); err != nil {
				t.Fatalf("recover: %v", err)
			}
			arc := submitAndAssociate(t, app, publicArc("STA-ALPHA", "CTX-"+tt.name, 0, 121))
			target, _, _, _, err := app.Association.Target(arc.AssociatedTargetID)
			if err != nil {
				t.Fatalf("get target: %v", err)
			}

			body, err := json.Marshal(map[string]any{
				"expected_association_revision": target.AssociationRevision,
				"run_now":                       true,
			})
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
			ctx, cancel := tt.newContext()
			defer cancel()
			req := httptest.NewRequest(http.MethodPost, "/v1/targets/"+target.ID+"/solve-jobs", bytes.NewReader(body)).WithContext(ctx)
			postRecorder := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				app.Handler().ServeHTTP(postRecorder, req)
				close(done)
			}()

			<-engine.started
			if tt.cancelNow {
				cancel()
			}
			select {
			case <-done:
			case <-time.After(250 * time.Millisecond):
				t.Fatal("run_now handler did not stop promptly after request cancellation")
			}
			if attached := <-engine.observed; !attached {
				t.Fatal("solve engine did not inherit the HTTP request context")
			}
			if !errors.Is(ctx.Err(), tt.wantErr) {
				t.Fatalf("request context error = %v, want %v", ctx.Err(), tt.wantErr)
			}
			if postRecorder.Code != http.StatusAccepted {
				t.Fatalf("POST status = %d, body = %s", postRecorder.Code, postRecorder.Body.String())
			}

			var posted domain.SolveJob
			if err := json.Unmarshal(postRecorder.Body.Bytes(), &posted); err != nil {
				t.Fatalf("decode POST response: %v", err)
			}
			getRecorder := httptest.NewRecorder()
			getReq := httptest.NewRequest(http.MethodGet, "/v1/solve-jobs/"+posted.ID, nil)
			app.Handler().ServeHTTP(getRecorder, getReq)
			if getRecorder.Code != http.StatusOK {
				t.Fatalf("GET status = %d, body = %s", getRecorder.Code, getRecorder.Body.String())
			}
			var got domain.SolveJob
			if err := json.Unmarshal(getRecorder.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode GET response: %v", err)
			}
			if got.Status != tt.wantStatus || got.FailureClass != tt.wantClass || got.ResultSolutionID != "" {
				t.Fatalf("persisted job = status %q, class %q, solution %q; want status %q, class %q, no solution", got.Status, got.FailureClass, got.ResultSolutionID, tt.wantStatus, tt.wantClass)
			}
		})
	}
}
