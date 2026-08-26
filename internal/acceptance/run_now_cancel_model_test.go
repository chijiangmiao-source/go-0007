package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"leo-debris-orbit-loop/internal/api"
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/orbit"
	"leo-debris-orbit-loop/internal/persistence"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type modelCancelEngine struct {
	entered  chan struct{}
	release  chan struct{}
	canceled chan struct{}
	once     sync.Once
}

func (e *modelCancelEngine) Version() string { return "model-cancel-v1" }

func (e *modelCancelEngine) Compute(ctx context.Context, _ orbit.InputSnapshot) (orbit.EngineResult, error) {
	e.once.Do(func() { close(e.entered) })
	select {
	case <-ctx.Done():
		close(e.canceled)
		return orbit.EngineResult{}, ctx.Err()
	case <-e.release:
		return orbit.EngineResult{Epoch: time.Unix(0, 0).UTC(), OutputHash: "completed-output"}, nil
	}
}

func TestModel_RunNowCancellation(t *testing.T) {
	cases := []struct {
		name               string
		cancelWhileRunning bool
		wantStatus         domain.SolveJobStatus
		wantApplied        bool
		wantCanceledEvent  int
		wantSuccessEvent   int
		wantSolutions      int
	}{
		{
			name:               "cancel_commit_interrupts_running_compute",
			cancelWhileRunning: true,
			wantStatus:         domain.JobCanceled,
			wantApplied:        true,
			wantCanceledEvent:  1,
		},
		{
			name:             "completed_terminal_state_wins_late_cancel",
			wantStatus:       domain.JobSucceeded,
			wantSuccessEvent: 1,
			wantSolutions:    1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := &modelCancelEngine{
				entered:  make(chan struct{}),
				release:  make(chan struct{}),
				canceled: make(chan struct{}),
			}
			app := api.NewApp(t.TempDir()+"/state.json", engine)
			if err := app.Recovery.Recover(); err != nil {
				t.Fatalf("recover: %v", err)
			}
			arc := submitAndAssociate(t, app, publicArc("STA-ALPHA", "MODEL-CANCEL", 0, 121.1))
			target, _, _, _, err := app.Association.Target(arc.AssociatedTargetID)
			if err != nil {
				t.Fatalf("target: %v", err)
			}

			handler := app.Handler()
			runBody := fmt.Sprintf(`{"expected_association_revision":%d,"run_now":true}`, target.AssociationRevision)
			runRequest := httptest.NewRequest(http.MethodPost, "/v1/targets/"+target.ID+"/solve-jobs", strings.NewReader(runBody))
			runResponse := httptest.NewRecorder()
			runDone := make(chan struct{})
			go func() {
				handler.ServeHTTP(runResponse, runRequest)
				close(runDone)
			}()

			select {
			case <-engine.entered:
			case <-time.After(time.Second):
				t.Fatal("run_now request did not enter Engine.Compute")
			}
			state, err := app.Store.Load()
			if err != nil {
				t.Fatalf("load running state: %v", err)
			}
			if len(state.SolveJobs) != 1 {
				t.Fatalf("expected one running job, got %d", len(state.SolveJobs))
			}
			var jobID string
			for id, job := range state.SolveJobs {
				jobID = id
				if job.Status != domain.JobRunning {
					t.Fatalf("job status before race = %s, want running", job.Status)
				}
			}

			if !tc.cancelWhileRunning {
				close(engine.release)
				select {
				case <-runDone:
				case <-time.After(time.Second):
					t.Fatal("completed run_now request did not return")
				}
			}
			cancelRequest := httptest.NewRequest(http.MethodPost, "/v1/solve-jobs/"+jobID+"/cancel", nil)
			cancelResponse := httptest.NewRecorder()
			handler.ServeHTTP(cancelResponse, cancelRequest)
			if cancelResponse.Code != http.StatusOK {
				t.Fatalf("cancel status = %d, body=%s", cancelResponse.Code, cancelResponse.Body.String())
			}
			var cancelResult orbit.CancelResult
			if err := json.NewDecoder(cancelResponse.Body).Decode(&cancelResult); err != nil {
				t.Fatalf("decode cancel response: %v", err)
			}
			if cancelResult.Applied != tc.wantApplied {
				t.Fatalf("cancel applied = %v, want %v", cancelResult.Applied, tc.wantApplied)
			}

			if tc.cancelWhileRunning {
				select {
				case <-engine.canceled:
				case <-time.After(250 * time.Millisecond):
					close(engine.release)
					<-runDone
					t.Fatal("Engine.Compute context was not promptly canceled")
				}
				select {
				case <-runDone:
				case <-time.After(250 * time.Millisecond):
					t.Fatal("run_now request did not return after cancellation")
				}
			}
			if runResponse.Code != http.StatusAccepted {
				t.Fatalf("run_now status = %d, body=%s", runResponse.Code, runResponse.Body.String())
			}

			secondCancel := httptest.NewRecorder()
			handler.ServeHTTP(secondCancel, httptest.NewRequest(http.MethodPost, "/v1/solve-jobs/"+jobID+"/cancel", nil))
			var secondResult orbit.CancelResult
			if secondCancel.Code != http.StatusOK || json.NewDecoder(secondCancel.Body).Decode(&secondResult) != nil || secondResult.Applied {
				t.Fatalf("terminal cancel was not idempotent: status=%d body=%s", secondCancel.Code, secondCancel.Body.String())
			}

			state, err = app.Store.Load()
			if err != nil {
				t.Fatalf("load final state: %v", err)
			}
			job := state.SolveJobs[jobID]
			if job.Status != tc.wantStatus {
				t.Fatalf("final job status = %s, want %s", job.Status, tc.wantStatus)
			}
			canceledEvents, successEvents := 0, 0
			for _, event := range state.Events {
				if event.AggregateID != jobID {
					continue
				}
				switch event.Type {
				case persistence.EventSolveCanceled:
					canceledEvents++
				case persistence.EventSolveSucceeded:
					successEvents++
				}
			}
			if canceledEvents != tc.wantCanceledEvent || successEvents != tc.wantSuccessEvent || len(state.Solutions) != tc.wantSolutions {
				t.Fatalf("terminal artifacts: canceled_events=%d succeeded_events=%d solutions=%d", canceledEvents, successEvents, len(state.Solutions))
			}
		})
	}
}
