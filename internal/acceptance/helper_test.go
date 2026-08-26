package acceptance

import (
	"context"
	"leo-debris-orbit-loop/internal/api"
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/intake"
	"leo-debris-orbit-loop/internal/orbit"
	"testing"
)

func newTestApp(t *testing.T, mode string) *api.App {
	t.Helper()
	app := api.NewApp(t.TempDir()+"/state.json", orbit.NewDeterministicEngine(mode))
	if err := app.Recovery.Recover(); err != nil {
		t.Fatalf("recover: %v", err)
	}
	return app
}

func publicArc(station, arc string, offset int, az float64) intake.SubmitArcRequest {
	base := []string{
		"2026-08-25T23:59:58.000Z",
		"2026-08-26T00:00:04.000Z",
		"2026-08-26T00:00:09.000Z",
	}
	samples := make([]intake.SampleRequest, 0, 3)
	for i, ts := range base {
		samples = append(samples, intake.SampleRequest{Time: ts, AzimuthDeg: az + float64(i)*0.3 + float64(offset)*0.01, ElevationDeg: 41.2 - float64(i)*0.2})
	}
	return intake.SubmitArcRequest{StationID: station, ArcID: arc, Confidence: 0.94, Samples: samples}
}

func submitAndAssociate(t *testing.T, app *api.App, req intake.SubmitArcRequest) domain.ObservationArc {
	t.Helper()
	res, err := app.Intake.SubmitArc(req)
	if err != nil {
		t.Fatalf("submit arc: %v", err)
	}
	if !res.Accepted {
		t.Fatalf("arc was not accepted")
	}
	if err := app.Association.ProcessPending(); err != nil {
		t.Fatalf("associate: %v", err)
	}
	arc, err := app.Intake.GetArc(res.ArcKey)
	if err != nil {
		t.Fatalf("get arc: %v", err)
	}
	return arc
}

func solveAndReview(t *testing.T, app *api.App, targetID string, revision int64) (domain.SolveJob, domain.ResidualReview) {
	t.Helper()
	job, err := app.Scheduler.CreateJob(targetID, revision)
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := app.Scheduler.RunJob(context.Background(), job.ID); err != nil {
		t.Fatalf("run job: %v", err)
	}
	job, err = app.Scheduler.GetJob(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if job.ResultSolutionID == "" {
		t.Fatalf("job has no solution: %+v", job)
	}
	review, err := app.Quality.EvaluateSolution(job.ResultSolutionID)
	if err != nil {
		t.Fatalf("quality: %v", err)
	}
	return job, review
}
