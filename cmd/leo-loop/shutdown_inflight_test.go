package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"leo-debris-orbit-loop/internal/api"
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/intake"
	"leo-debris-orbit-loop/internal/orbit"
	"leo-debris-orbit-loop/internal/persistence"
	"net"
	"net/http"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
)

type shutdownGateEngine struct {
	started chan struct{}
	release chan struct{}
}

func (e *shutdownGateEngine) Version() string { return "shutdown-gate-v1" }

func (e *shutdownGateEngine) Compute(ctx context.Context, snapshot orbit.InputSnapshot) (orbit.EngineResult, error) {
	close(e.started)
	select {
	case <-e.release:
		return orbit.NewDeterministicEngine("normal").Compute(ctx, snapshot)
	case <-ctx.Done():
		return orbit.EngineResult{}, ctx.Err()
	}
}

func TestModel_ShutdownWaitsForInflightRunNow(t *testing.T) {
	cases := []struct {
		name   string
		signal os.Signal
	}{
		{name: "SIGTERM after solve is running", signal: syscall.SIGTERM},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Go cannot deliver console control events through Process.Signal on
			// Windows. Keep that platform covered by checking the serve entry point,
			// while exercising the complete wire behavior where SIGTERM is supported.
			if runtime.GOOS == "windows" {
				file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
				if err != nil {
					t.Fatalf("parse serve entry point: %v", err)
				}
				usesShutdown := false
				for _, decl := range file.Decls {
					fn, ok := decl.(*ast.FuncDecl)
					if !ok || fn.Name.Name != "serve" {
						continue
					}
					ast.Inspect(fn.Body, func(n ast.Node) bool {
						call, ok := n.(*ast.CallExpr)
						if !ok {
							return true
						}
						sel, ok := call.Fun.(*ast.SelectorExpr)
						if ok && sel.Sel.Name == "Shutdown" {
							usesShutdown = true
						}
						return true
					})
				}
				if !usesShutdown {
					t.Fatal("serve does not gracefully shut down its HTTP server")
				}
				return
			}

			gate := &shutdownGateEngine{started: make(chan struct{}), release: make(chan struct{})}
			app := api.NewApp(t.TempDir()+"/state.json", gate)
			if err := app.Recovery.Recover(); err != nil {
				t.Fatalf("recover initial state: %v", err)
			}
			arc, err := app.Intake.SubmitArc(intake.SubmitArcRequest{
				StationID:  "STA-ALPHA",
				ArcID:      "SHUTDOWN-ARC",
				Confidence: 0.94,
				Samples: []intake.SampleRequest{
					{Time: "2026-08-25T23:59:58.000Z", AzimuthDeg: 121.1, ElevationDeg: 41.2},
					{Time: "2026-08-26T00:00:04.000Z", AzimuthDeg: 121.4, ElevationDeg: 41.0},
					{Time: "2026-08-26T00:00:09.000Z", AzimuthDeg: 121.8, ElevationDeg: 40.8},
				},
			})
			if err != nil {
				t.Fatalf("submit arc: %v", err)
			}
			if err := app.Association.ProcessPending(); err != nil {
				t.Fatalf("associate arc: %v", err)
			}
			associated, err := app.Intake.GetArc(arc.ArcKey)
			if err != nil {
				t.Fatalf("get associated arc: %v", err)
			}
			target, _, _, _, err := app.Association.Target(associated.AssociatedTargetID)
			if err != nil {
				t.Fatalf("get target: %v", err)
			}

			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("reserve address: %v", err)
			}
			addr := listener.Addr().String()
			if err := listener.Close(); err != nil {
				t.Fatalf("release address: %v", err)
			}
			serveDone := make(chan error, 1)
			go func() { serveDone <- serve(app, addr, 2*time.Second) }()

			client := &http.Client{Timeout: 3 * time.Second}
			baseURL := "http://" + addr
			readyDeadline := time.Now().Add(2 * time.Second)
			for {
				resp, getErr := client.Get(baseURL + "/healthz")
				if getErr == nil {
					_ = resp.Body.Close()
					break
				}
				if time.Now().After(readyDeadline) {
					t.Fatalf("server did not become ready: %v", getErr)
				}
				time.Sleep(5 * time.Millisecond)
			}

			payload := fmt.Sprintf(`{"expected_association_revision":%d,"run_now":true}`, target.AssociationRevision)
			type httpResult struct {
				status int
				body   []byte
				err    error
			}
			requestDone := make(chan httpResult, 1)
			go func() {
				resp, requestErr := client.Post(baseURL+"/v1/targets/"+target.ID+"/solve-jobs", "application/json", bytes.NewBufferString(payload))
				if requestErr != nil {
					requestDone <- httpResult{err: requestErr}
					return
				}
				body, readErr := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				requestDone <- httpResult{status: resp.StatusCode, body: body, err: readErr}
			}()

			select {
			case <-gate.started:
			case <-time.After(2 * time.Second):
				t.Fatal("run_now solve never entered the engine")
			}
			process, err := os.FindProcess(os.Getpid())
			if err != nil {
				t.Fatalf("find test process: %v", err)
			}
			if err := process.Signal(tc.signal); err != nil {
				close(gate.release)
				t.Fatalf("send %s: %v", tc.signal, err)
			}

			select {
			case err := <-serveDone:
				close(gate.release)
				t.Fatalf("serve returned before its active handler completed: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			close(gate.release)

			result := <-requestDone
			if result.err != nil {
				t.Fatalf("run_now response was interrupted: %v", result.err)
			}
			if result.status != http.StatusAccepted {
				t.Fatalf("run_now status = %d, body = %s", result.status, result.body)
			}
			var responseJob domain.SolveJob
			if err := json.Unmarshal(result.body, &responseJob); err != nil {
				t.Fatalf("decode complete run_now response: %v; body = %s", err, result.body)
			}
			if responseJob.Status != domain.JobSucceeded || responseJob.ResultSolutionID == "" {
				t.Fatalf("response job is not a committed success: %+v", responseJob)
			}
			if err := <-serveDone; err != nil {
				t.Fatalf("serve after graceful shutdown: %v", err)
			}

			state, err := app.Store.Load()
			if err != nil {
				t.Fatalf("load committed state: %v", err)
			}
			storedJob := state.SolveJobs[responseJob.ID]
			if storedJob.Status != domain.JobSucceeded || storedJob.ResultSolutionID != responseJob.ResultSolutionID {
				t.Fatalf("stored job disagrees with HTTP response: %+v", storedJob)
			}
			if _, ok := state.Solutions[responseJob.ResultSolutionID]; !ok {
				t.Fatalf("solution %q was not committed", responseJob.ResultSolutionID)
			}
			wantEvents := []string{persistence.EventSolveQueued, persistence.EventSolveRunning, persistence.EventSolveSucceeded, persistence.EventResidualReviewed}
			for _, want := range wantEvents {
				found := false
				for _, event := range state.Events {
					if event.Type == want && (event.AggregateID == responseJob.ID || want == persistence.EventResidualReviewed) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing committed event %q", want)
				}
			}
			if state.Checkpoint.LastAppliedSeq != int64(len(state.Events)) || state.Checkpoint.StateHash == "" {
				t.Errorf("checkpoint is not aligned with committed events: %+v, events=%d", state.Checkpoint, len(state.Events))
			}
		})
	}
}
