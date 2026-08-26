package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"leo-debris-orbit-loop/internal/domain"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestModel_QueuedSolveExecutionDeadline(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "queued time does not consume execution budget and reuse stays idempotent",
			run: func(t *testing.T) {
				app := newTestApp(t, "normal")
				arc := submitAndAssociate(t, app, publicArc("STA-ALPHA", "QUEUED-DEADLINE", 0, 121))
				target, _, _, _, err := app.Association.Target(arc.AssociatedTargetID)
				if err != nil {
					t.Fatalf("get target: %v", err)
				}

				post := func(runNow bool) domain.SolveJob {
					t.Helper()
					body := fmt.Sprintf(`{"expected_association_revision":%d,"run_now":%t}`, target.AssociationRevision, runNow)
					req := httptest.NewRequest(http.MethodPost, "/v1/targets/"+target.ID+"/solve-jobs", bytes.NewBufferString(body))
					rec := httptest.NewRecorder()
					app.Handler().ServeHTTP(rec, req)
					if rec.Code != http.StatusAccepted {
						t.Fatalf("create solve job returned %d: %s", rec.Code, rec.Body.String())
					}
					var job domain.SolveJob
					if err := json.NewDecoder(rec.Body).Decode(&job); err != nil {
						t.Fatalf("decode solve job: %v", err)
					}
					return job
				}

				queued := post(false)
				if queued.Status != domain.JobQueued {
					t.Fatalf("initial job status = %s, want %s", queued.Status, domain.JobQueued)
				}
				if delay := time.Until(queued.Deadline) + 50*time.Millisecond; delay > 0 {
					time.Sleep(delay)
				}

				started := post(true)
				if started.ID != queued.ID {
					t.Fatalf("run_now created job %q, want queued job %q", started.ID, queued.ID)
				}
				if started.Status != domain.JobSucceeded || started.ResultSolutionID == "" {
					t.Fatalf("job did not solve after waiting in queue: %+v", started)
				}
				if !started.Deadline.After(queued.Deadline) {
					t.Fatalf("execution deadline %s was not reset after queued deadline %s", started.Deadline, queued.Deadline)
				}

				again := post(true)
				if again.ID != queued.ID || again.ResultSolutionID != started.ResultSolutionID {
					t.Fatalf("repeated request was not idempotent: first=%+v repeated=%+v", started, again)
				}
				state, err := app.Store.Load()
				if err != nil {
					t.Fatalf("load state: %v", err)
				}
				if len(state.SolveJobs) != 1 || len(state.Solutions) != 1 {
					t.Fatalf("repeated request created duplicates: jobs=%d solutions=%d", len(state.SolveJobs), len(state.Solutions))
				}
			},
		},
		{
			name: "execution exceeding job timeout remains timed out",
			run: func(t *testing.T) {
				app := newTestApp(t, "timeout")
				arc := submitAndAssociate(t, app, publicArc("STA-ALPHA", "EXECUTION-TIMEOUT", 0, 122))
				target, _, _, _, err := app.Association.Target(arc.AssociatedTargetID)
				if err != nil {
					t.Fatalf("get target: %v", err)
				}
				job, err := app.Scheduler.CreateJob(target.ID, target.AssociationRevision)
				if err != nil {
					t.Fatalf("create job: %v", err)
				}
				if err := app.Scheduler.RunJob(context.Background(), job.ID); err != nil {
					t.Fatalf("run job: %v", err)
				}
				got, err := app.Scheduler.GetJob(job.ID)
				if err != nil {
					t.Fatalf("get job: %v", err)
				}
				if got.Status != domain.JobTimedOut || got.FailureClass != string(domain.CodeTimeout) {
					t.Fatalf("execution timeout classification changed: %+v", got)
				}
			},
		},
		{
			name: "earlier caller deadline is respected",
			run: func(t *testing.T) {
				app := newTestApp(t, "timeout")
				arc := submitAndAssociate(t, app, publicArc("STA-ALPHA", "CALLER-DEADLINE", 0, 123))
				target, _, _, _, err := app.Association.Target(arc.AssociatedTargetID)
				if err != nil {
					t.Fatalf("get target: %v", err)
				}
				job, err := app.Scheduler.CreateJob(target.ID, target.AssociationRevision)
				if err != nil {
					t.Fatalf("create job: %v", err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
				defer cancel()
				startedAt := time.Now()
				if err := app.Scheduler.RunJob(ctx, job.ID); err != nil {
					t.Fatalf("run job: %v", err)
				}
				if elapsed := time.Since(startedAt); elapsed >= 500*time.Millisecond {
					t.Fatalf("caller deadline was ignored; run took %s", elapsed)
				}
				got, err := app.Scheduler.GetJob(job.ID)
				if err != nil {
					t.Fatalf("get job: %v", err)
				}
				if got.Status != domain.JobTimedOut || got.FailureClass != string(domain.CodeTimeout) {
					t.Fatalf("caller deadline classification changed: %+v", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}
