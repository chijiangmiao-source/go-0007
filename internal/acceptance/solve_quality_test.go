package acceptance

import (
	"context"
	"leo-debris-orbit-loop/internal/api"
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/orbit"
	"leo-debris-orbit-loop/internal/quality"
	"testing"
)

func TestDV08CancelQueuedJobWinsBeforeTerminalState(t *testing.T) {
	app := newTestApp(t, "normal")
	arc := submitAndAssociate(t, app, publicArc("STA-ALPHA", "CANCEL-1", 0, 121))
	target, _, _, _, err := app.Association.Target(arc.AssociatedTargetID)
	if err != nil {
		t.Fatal(err)
	}
	job, err := app.Scheduler.CreateJob(target.ID, target.AssociationRevision)
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.Scheduler.Cancel(job.ID, "test cancel")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || result.FinalStatus != string(domain.JobCanceled) {
		t.Fatalf("cancel did not apply: %+v", result)
	}
	if err := app.Scheduler.RunJob(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := app.Scheduler.GetJob(job.ID)
	if got.Status != domain.JobCanceled {
		t.Fatalf("late run changed terminal cancel: %s", got.Status)
	}
}

func TestDV09NonConvergenceIsTerminalFailure(t *testing.T) {
	app := newTestApp(t, "nonconverge")
	arc := submitAndAssociate(t, app, publicArc("STA-ALPHA", "NC-1", 0, 121))
	target, _, _, _, _ := app.Association.Target(arc.AssociatedTargetID)
	job, err := app.Scheduler.CreateJob(target.ID, target.AssociationRevision)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Scheduler.RunJob(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := app.Scheduler.GetJob(job.ID)
	if got.Status != domain.JobFailed || got.FailureClass != string(domain.CodeNonConverged) || got.ResultSolutionID != "" {
		t.Fatalf("unexpected nonconverged job: %+v", got)
	}
}

func TestDV10RetryAndTimeoutClassificationsAreBounded(t *testing.T) {
	retryApp := newTestApp(t, "retry")
	arc := submitAndAssociate(t, retryApp, publicArc("STA-ALPHA", "RETRY-1", 0, 121))
	target, _, _, _, _ := retryApp.Association.Target(arc.AssociatedTargetID)
	job, _ := retryApp.Scheduler.CreateJob(target.ID, target.AssociationRevision)
	if err := retryApp.Scheduler.RunJob(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := retryApp.Scheduler.GetJob(job.ID)
	if got.Attempts != 3 || got.FailureClass != string(domain.CodeRetryExhausted) {
		t.Fatalf("retry not bounded: attempts=%d class=%s", got.Attempts, got.FailureClass)
	}
	timeoutApp := newTestApp(t, "timeout")
	arc = submitAndAssociate(t, timeoutApp, publicArc("STA-ALPHA", "TIMEOUT-1", 0, 121))
	target, _, _, _, _ = timeoutApp.Association.Target(arc.AssociatedTargetID)
	job, _ = timeoutApp.Scheduler.CreateJob(target.ID, target.AssociationRevision)
	if err := timeoutApp.Scheduler.RunJob(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = timeoutApp.Scheduler.GetJob(job.ID)
	if got.Status != domain.JobTimedOut || got.FailureClass != string(domain.CodeTimeout) {
		t.Fatalf("timeout not classified: %+v", got)
	}
}

func TestDV11LowResidualAutoApproves(t *testing.T) {
	app := newTestApp(t, "normal")
	arc := submitAndAssociate(t, app, publicArc("STA-ALPHA", "QA-LOW", 0, 121))
	target, _, _, _, _ := app.Association.Target(arc.AssociatedTargetID)
	job, review := solveAndReview(t, app, target.ID, target.AssociationRevision)
	if job.Status != domain.JobSucceeded || review.Status != domain.ReviewApproved {
		t.Fatalf("expected success and auto approval: job=%s review=%s", job.Status, review.Status)
	}
	if review.Summary.SampleCount != 3 || review.Summary.RMSDeg == 0 {
		t.Fatalf("residual summary not preserved: %+v", review.Summary)
	}
}

func TestDV12ReviewBandAndRejectBandEnforceTransitions(t *testing.T) {
	reviewApp := newTestApp(t, "review")
	arc := submitAndAssociate(t, reviewApp, publicArc("STA-ALPHA", "QA-REVIEW", 0, 121))
	target, _, _, _, _ := reviewApp.Association.Target(arc.AssociatedTargetID)
	job, review := solveAndReview(t, reviewApp, target.ID, target.AssociationRevision)
	if review.Status != domain.ReviewPending {
		t.Fatalf("expected pending review, got %s", review.Status)
	}
	if _, err := reviewApp.Quality.Decide(job.ResultSolutionID, quality.DecisionRequest{Decision: "approve", Reason: "operator accepted"}); err != nil {
		t.Fatalf("approve pending review: %v", err)
	}
	rejectApp := api.NewApp(t.TempDir()+"/state.json", orbit.NewDeterministicEngine("reject"))
	arc = submitAndAssociate(t, rejectApp, publicArc("STA-ALPHA", "QA-REJECT", 0, 121))
	target, _, _, _, _ = rejectApp.Association.Target(arc.AssociatedTargetID)
	job, review = solveAndReview(t, rejectApp, target.ID, target.AssociationRevision)
	if review.Status != domain.ReviewRejected {
		t.Fatalf("expected direct reject, got %s", review.Status)
	}
	if _, err := rejectApp.Quality.Decide(job.ResultSolutionID, quality.DecisionRequest{Decision: "approve", Reason: "illegal"}); !domain.IsCode(err, domain.CodeIllegalState) {
		t.Fatalf("expected illegal transition, got %v", err)
	}
}
