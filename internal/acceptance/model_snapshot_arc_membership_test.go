package acceptance

import (
	"context"
	"leo-debris-orbit-loop/internal/api"
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/orbit"
	"leo-debris-orbit-loop/internal/versioning"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type snapshotGateEngine struct {
	delegate    orbit.Engine
	started     chan orbit.InputSnapshot
	release     chan struct{}
	releaseOnce sync.Once
	calls       atomic.Int32
}

func newSnapshotGateEngine() *snapshotGateEngine {
	return &snapshotGateEngine{
		delegate: orbit.NewDeterministicEngine("normal"),
		started:  make(chan orbit.InputSnapshot, 1),
		release:  make(chan struct{}),
	}
}

func (e *snapshotGateEngine) Version() string { return e.delegate.Version() }

func (e *snapshotGateEngine) Compute(ctx context.Context, snapshot orbit.InputSnapshot) (orbit.EngineResult, error) {
	if e.calls.Add(1) == 1 {
		e.started <- snapshot
		select {
		case <-e.release:
		case <-ctx.Done():
			return orbit.EngineResult{}, ctx.Err()
		}
	}
	return e.delegate.Compute(ctx, snapshot)
}

func (e *snapshotGateEngine) unblock() { e.releaseOnce.Do(func() { close(e.release) }) }

type snapshotRaceResult struct {
	app         *api.App
	engine      *snapshotGateEngine
	targetID    string
	firstJob    domain.SolveJob
	first       domain.OrbitSolution
	input       orbit.InputSnapshot
	inputArcIDs []string
	lateArcID   string
}

func runSnapshotRace(t *testing.T) snapshotRaceResult {
	t.Helper()
	engine := newSnapshotGateEngine()
	t.Cleanup(engine.unblock)
	app := api.NewApp(t.TempDir()+"/state.json", engine)
	if err := app.Recovery.Recover(); err != nil {
		t.Fatalf("recover: %v", err)
	}
	initial := submitAndAssociate(t, app, publicArc("STA-ALPHA", "SNAPSHOT-OLD", 0, 121))
	target, _, _, _, err := app.Association.Target(initial.AssociatedTargetID)
	if err != nil {
		t.Fatalf("get initial target: %v", err)
	}
	job, err := app.Scheduler.CreateJob(target.ID, target.AssociationRevision)
	if err != nil {
		t.Fatalf("create first job: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- app.Scheduler.RunJob(context.Background(), job.ID) }()

	var input orbit.InputSnapshot
	select {
	case input = <-engine.started:
	case err := <-runDone:
		t.Fatalf("job returned before entering engine: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("job did not enter engine")
	}
	late := submitAndAssociate(t, app, publicArc("STA-ALPHA", "SNAPSHOT-LATE", 1, 121.1))
	if late.AssociatedTargetID != target.ID {
		t.Fatalf("late arc associated to %q, want %q", late.AssociatedTargetID, target.ID)
	}
	engine.unblock()
	if err := <-runDone; err != nil {
		t.Fatalf("run first job: %v", err)
	}
	job, err = app.Scheduler.GetJob(job.ID)
	if err != nil {
		t.Fatalf("get completed job: %v", err)
	}
	if job.Status != domain.JobSucceeded || job.ResultSolutionID == "" {
		t.Fatalf("first job did not succeed: %+v", job)
	}
	state, err := app.Store.Load()
	if err != nil {
		t.Fatalf("load public state: %v", err)
	}
	solution, ok := state.Solutions[job.ResultSolutionID]
	if !ok {
		t.Fatalf("solution %q not found", job.ResultSolutionID)
	}
	inputIDs := make([]string, 0, len(input.Arcs))
	for _, arc := range input.Arcs {
		inputIDs = append(inputIDs, arc.ArcID)
	}
	return snapshotRaceResult{app: app, engine: engine, targetID: target.ID, firstJob: job, first: solution, input: input, inputArcIDs: inputIDs, lateArcID: late.ID}
}

func TestModel_SolveSnapshotArcMembershipRemainsConsistent(t *testing.T) {
	cases := []struct {
		name  string
		check func(*testing.T, snapshotRaceResult)
	}{
		{
			name: "solution fields all describe the snapshot passed to the engine",
			check: func(t *testing.T, got snapshotRaceResult) {
				if len(got.inputArcIDs) != 1 || got.inputArcIDs[0] == got.lateArcID {
					t.Fatalf("engine snapshot was not isolated from late arc: %v", got.inputArcIDs)
				}
				if hash := domain.MustHashAny(got.input); hash != got.firstJob.InputSnapshotHash {
					t.Fatalf("executed snapshot hash = %q, job hash = %q", hash, got.firstJob.InputSnapshotHash)
				}
				if !reflect.DeepEqual(got.first.ObservationArcIDs, got.inputArcIDs) {
					t.Fatalf("solution arc IDs = %v, engine snapshot arc IDs = %v", got.first.ObservationArcIDs, got.inputArcIDs)
				}
				expected, err := orbit.NewDeterministicEngine("normal").Compute(context.Background(), got.input)
				if err != nil {
					t.Fatalf("compute expected snapshot result: %v", err)
				}
				if got.first.OutputHash != expected.OutputHash || !reflect.DeepEqual(got.first.Parameters, expected.Params) || !reflect.DeepEqual(got.first.Residuals, expected.Residuals) {
					t.Fatalf("solution output is not wholly derived from its engine snapshot: %+v", got.first)
				}
			},
		},
		{
			name: "frozen version stays on the old snapshot and late arc enters the next job",
			check: func(t *testing.T, got snapshotRaceResult) {
				review, err := got.app.Quality.EvaluateSolution(got.first.ID)
				if err != nil || review.Status != domain.ReviewApproved {
					t.Fatalf("approve first solution: review=%+v err=%v", review, err)
				}
				frozen, err := got.app.Versioning.Freeze(got.targetID, versioning.FreezeRequest{SolutionID: got.first.ID, ExpectedCurrentVersion: 0})
				if err != nil {
					t.Fatalf("freeze first solution: %v", err)
				}
				if !reflect.DeepEqual(frozen.InputArcIDs, got.inputArcIDs) || frozen.ResultHash != got.first.OutputHash {
					t.Fatalf("frozen inputs/result diverged from solved snapshot: %+v", frozen)
				}

				target, arcs, _, _, err := got.app.Association.Target(got.targetID)
				if err != nil {
					t.Fatalf("query updated target: %v", err)
				}
				if len(arcs) != 2 {
					t.Fatalf("public target query arc count = %d, want 2", len(arcs))
				}
				nextJob, err := got.app.Scheduler.CreateJob(target.ID, target.AssociationRevision)
				if err != nil {
					t.Fatalf("create next job: %v", err)
				}
				if err := got.app.Scheduler.RunJob(context.Background(), nextJob.ID); err != nil {
					t.Fatalf("run next job: %v", err)
				}
				nextJob, err = got.app.Scheduler.GetJob(nextJob.ID)
				if err != nil {
					t.Fatalf("get next job: %v", err)
				}
				state, err := got.app.Store.Load()
				if err != nil {
					t.Fatalf("load state after next job: %v", err)
				}
				next := state.Solutions[nextJob.ResultSolutionID]
				if len(next.ObservationArcIDs) != 2 || !containsString(next.ObservationArcIDs, got.lateArcID) {
					t.Fatalf("next solution arc IDs = %v, want late arc %q included", next.ObservationArcIDs, got.lateArcID)
				}
				after, err := got.app.Versioning.Get(got.targetID, frozen.Version)
				if err != nil {
					t.Fatalf("get frozen version: %v", err)
				}
				if !reflect.DeepEqual(after, frozen) {
					t.Fatalf("later solve mutated frozen version: before=%+v after=%+v", frozen, after)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.check(t, runSnapshotRace(t)) })
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
