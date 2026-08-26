package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"leo-debris-orbit-loop/internal/api"
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/orbit"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type controlledFinishEngine struct {
	started chan struct{}
	release chan struct{}
	result  orbit.EngineResult
	err     error
}

func (e *controlledFinishEngine) Version() string { return "controlled-finish-v1" }

func (e *controlledFinishEngine) Compute(context.Context, orbit.InputSnapshot) (orbit.EngineResult, error) {
	close(e.started)
	<-e.release
	return e.result, e.err
}

func TestModel_CanceledRunningJobIgnoresLateEngineOutcome(t *testing.T) {
	cases := []struct {
		name   string
		result orbit.EngineResult
		err    error
	}{
		{
			name: "success",
			result: orbit.EngineResult{
				Epoch:      time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC),
				OutputHash: "late-success",
				Iteration:  domain.IterationSummary{Iterations: 1, Converged: true},
			},
		},
		{name: "timeout", err: context.DeadlineExceeded},
		{name: "failure", err: domain.NewError(domain.CodeNonConverged, "late failure")},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := &controlledFinishEngine{
				started: make(chan struct{}),
				release: make(chan struct{}),
				result:  tc.result,
				err:     tc.err,
			}
			app := api.NewApp(t.TempDir()+"/state.json", engine)
			if err := app.Recovery.Recover(); err != nil {
				t.Fatalf("recover: %v", err)
			}
			arc := submitAndAssociate(t, app, publicArc("STA-ALPHA", fmt.Sprintf("CANCEL-RUNNING-%d", i), 0, 121))
			target, _, _, _, err := app.Association.Target(arc.AssociatedTargetID)
			if err != nil {
				t.Fatalf("get target: %v", err)
			}

			type response struct {
				status int
				body   []byte
			}
			runResponse := make(chan response, 1)
			go func() {
				body := bytes.NewBufferString(fmt.Sprintf(`{"expected_association_revision":%d,"run_now":true}`, target.AssociationRevision))
				req := httptest.NewRequest(http.MethodPost, "/v1/targets/"+target.ID+"/solve-jobs", body)
				recorder := httptest.NewRecorder()
				app.Handler().ServeHTTP(recorder, req)
				runResponse <- response{status: recorder.Code, body: recorder.Body.Bytes()}
			}()

			select {
			case <-engine.started:
			case <-time.After(2 * time.Second):
				t.Fatal("run_now did not enter the engine")
			}
			_, _, running, _, err := app.Association.Target(target.ID)
			if err != nil {
				t.Fatalf("get running job: %v", err)
			}
			if running == nil || running.Status != domain.JobRunning {
				t.Fatalf("job was not running at cancellation: %+v", running)
			}

			cancelReq := httptest.NewRequest(http.MethodPost, "/v1/solve-jobs/"+running.ID+"/cancel", nil)
			cancelRecorder := httptest.NewRecorder()
			app.Handler().ServeHTTP(cancelRecorder, cancelReq)
			if cancelRecorder.Code != http.StatusOK {
				t.Fatalf("cancel status = %d, body = %s", cancelRecorder.Code, cancelRecorder.Body.String())
			}
			var canceled orbit.CancelResult
			if err := json.NewDecoder(cancelRecorder.Body).Decode(&canceled); err != nil {
				t.Fatalf("decode cancel response: %v", err)
			}
			if !canceled.Applied || canceled.FinalStatus != string(domain.JobCanceled) {
				t.Fatalf("cancel response = %+v", canceled)
			}

			close(engine.release)
			var finished response
			select {
			case finished = <-runResponse:
			case <-time.After(2 * time.Second):
				t.Fatal("run_now did not finish after the engine was released")
			}
			if finished.status != http.StatusAccepted {
				t.Fatalf("run_now status = %d, body = %s", finished.status, finished.body)
			}
			var returned domain.SolveJob
			if err := json.Unmarshal(finished.body, &returned); err != nil {
				t.Fatalf("decode run_now response: %v", err)
			}
			if returned.Status != domain.JobCanceled {
				t.Fatalf("run_now returned status %s, want canceled", returned.Status)
			}

			settled, err := app.Scheduler.GetJob(running.ID)
			if err != nil {
				t.Fatalf("get settled job: %v", err)
			}
			if settled.Status != domain.JobCanceled || settled.ResultSolutionID != "" || settled.FailureClass != "" {
				t.Fatalf("late result mutated canceled job: %+v", settled)
			}
			state, err := app.Store.Load()
			if err != nil {
				t.Fatalf("load state: %v", err)
			}
			if len(state.Solutions) != 0 {
				t.Fatalf("late result created %d solutions", len(state.Solutions))
			}
			if settled.CancelEventSeq == 0 {
				t.Fatal("cancellation event was not retained")
			}
		})
	}
}
