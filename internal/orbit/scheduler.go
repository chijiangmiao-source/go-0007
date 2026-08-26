package orbit

import (
	"context"
	"errors"
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/persistence"
	"sync"
	"time"
)

type Scheduler struct {
	store       *persistence.Store
	engine      Engine
	jobTimeout  time.Duration
	maxAttempts int

	runMu   sync.Mutex
	runners map[string]context.CancelFunc
}

func NewScheduler(store *persistence.Store, engine Engine) *Scheduler {
	return &Scheduler{store: store, engine: engine, jobTimeout: 750 * time.Millisecond, maxAttempts: 3, runners: make(map[string]context.CancelFunc)}
}

// setRunner registers the cancel function for an in-flight computation so a
// concurrent Cancel can interrupt it. Registered before the running transition
// so a cancel that races the transition is never missed.
func (s *Scheduler) setRunner(jobID string, cancel context.CancelFunc) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.runners[jobID] = cancel
}

func (s *Scheduler) clearRunner(jobID string) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	delete(s.runners, jobID)
}

// cancelRunner signals an in-flight computation to stop without waiting for the
// deadline. It is only invoked after the cancel terminal state has committed,
// so the terminal-state race (first committed terminal wins) is preserved.
func (s *Scheduler) cancelRunner(jobID string) {
	s.runMu.Lock()
	cancel, ok := s.runners[jobID]
	s.runMu.Unlock()
	if ok {
		cancel()
	}
}

func (s *Scheduler) CreateJob(targetID string, expectedRevision int64) (domain.SolveJob, error) {
	var out domain.SolveJob
	err := s.store.Update(func(st *persistence.State) error {
		target, ok := st.Targets[targetID]
		if !ok {
			return domain.Errorf(domain.CodeNotFound, "target %s not found", targetID)
		}
		if target.AssociationRevision != expectedRevision {
			return domain.Errorf(domain.CodeConflict, "expected association revision %d but current is %d", expectedRevision, target.AssociationRevision)
		}
		for _, job := range st.SolveJobs {
			if job.TargetID == targetID && job.AssociationRevision == expectedRevision && (job.Status == domain.JobQueued || job.Status == domain.JobRunning || job.Status == domain.JobSucceeded) {
				out = job
				return nil
			}
		}
		arcs := persistence.ArcsForTarget(st, targetID)
		if len(arcs) == 0 {
			return domain.NewError(domain.CodePreconditionFail, "target has no associated arcs")
		}
		_, hash, err := BuildSnapshot(target, arcs, s.engine.Version())
		if err != nil {
			return err
		}
		job := domain.SolveJob{ID: persistence.NextJobID(st), TargetID: targetID, AssociationRevision: expectedRevision, InputSnapshotHash: hash, CalculatorVersion: s.engine.Version(), Status: domain.JobQueued, Deadline: domain.NowUTC().Add(s.jobTimeout), CreatedAt: domain.NowUTC(), UpdatedAt: domain.NowUTC()}
		ev := persistence.AppendEvent(st, persistence.EventSolveQueued, job.ID, hash, targetID)
		job.UpdatedAt = ev.RecordedAt
		st.SolveJobs[job.ID] = job
		out = job
		return nil
	})
	return out, err
}

func (s *Scheduler) RunJob(ctx context.Context, jobID string) error {
	runCtx, runCancel := context.WithCancel(ctx)
	s.setRunner(jobID, runCancel)
	defer runCancel()      // always release the derived context per context.WithCancel contract
	defer s.clearRunner(jobID)
	var snapshot InputSnapshot
	var job domain.SolveJob
	if err := s.store.Update(func(st *persistence.State) error {
		current, ok := st.SolveJobs[jobID]
		if !ok {
			return domain.Errorf(domain.CodeNotFound, "job %s not found", jobID)
		}
		if current.Status != domain.JobQueued {
			// A concurrent Cancel may have committed the canceled terminal
			// state before this transition; treat any terminal state (incl.
			// canceled) as "nothing to run" so a late cancel does not kick off
			// a computation that could produce a late result.
			if domain.IsTerminalJob(current.Status) {
				return nil
			}
			return domain.Errorf(domain.CodeIllegalState, "job %s is %s", jobID, current.Status)
		}
		target := st.Targets[current.TargetID]
		arcs := persistence.ArcsForTarget(st, current.TargetID)
		snap, hash, err := BuildSnapshot(target, arcs, s.engine.Version())
		if err != nil {
			return err
		}
		if hash != current.InputSnapshotHash {
			return domain.NewError(domain.CodeConflict, "input snapshot hash changed before execution")
		}
		current.Status = domain.JobRunning
		current.Attempts++
		ev := persistence.AppendEvent(st, persistence.EventSolveRunning, current.ID, current.InputSnapshotHash, "")
		current.UpdatedAt = ev.RecordedAt
		st.SolveJobs[current.ID] = current
		snapshot = snap
		job = current
		return nil
	}); err != nil {
		return err
	}
	if job.ID == "" {
		return nil
	}
	var result EngineResult
	var runErr error
	for {
		deadlineCtx, cancel := context.WithDeadline(runCtx, job.Deadline)
		result, runErr = s.engine.Compute(deadlineCtx, snapshot)
		cancel()
		if runErr == nil || !domain.IsCode(runErr, domain.CodeUnavailable) || job.Attempts >= s.maxAttempts {
			break
		}
		time.Sleep(time.Duration(job.Attempts) * 10 * time.Millisecond)
		job.Attempts++
	}
	return s.finishJob(jobID, job.Attempts, result, runErr)
}

func (s *Scheduler) finishJob(jobID string, attempts int, result EngineResult, runErr error) error {
	return s.store.Update(func(st *persistence.State) error {
		job := st.SolveJobs[jobID]
		if domain.IsTerminalJob(job.Status) {
			return nil
		}
		job.Attempts = attempts
		if runErr != nil {
			status, class := classifyRunError(runErr)
			if !domain.CanTransitionJob(job.Status, status) {
				return (&domain.AppError{Code: domain.CodeIllegalState, Message: "illegal solve transition", State: string(job.Status)})
			}
			job.Status = status
			job.FailureClass = class
			evType := persistence.EventSolveFailed
			if status == domain.JobCanceled {
				evType = persistence.EventSolveCanceled
			}
			ev := persistence.AppendEvent(st, evType, job.ID, job.InputSnapshotHash, class)
			job.UpdatedAt = ev.RecordedAt
			st.SolveJobs[job.ID] = job
			return nil
		}
		solution := domain.OrbitSolution{ID: persistence.NextSolutionID(st), JobID: job.ID, TargetID: job.TargetID, Epoch: result.Epoch, Parameters: result.Params, Iteration: result.Iteration, OutputHash: result.OutputHash, GeneratedAt: domain.NowUTC(), Residuals: append([]domain.ResidualPoint(nil), result.Residuals...)}
		for _, arc := range snapshotArcsForSolution(st, job.TargetID) {
			solution.ObservationArcIDs = append(solution.ObservationArcIDs, arc.ID)
		}
		job.Status = domain.JobSucceeded
		job.ResultSolutionID = solution.ID
		ev := persistence.AppendEvent(st, persistence.EventSolveSucceeded, job.ID, result.OutputHash, solution.ID)
		job.UpdatedAt = ev.RecordedAt
		st.Solutions[solution.ID] = solution
		st.SolveJobs[job.ID] = job
		return nil
	})
}

func classifyRunError(err error) (domain.SolveJobStatus, string) {
	if errors.Is(err, context.Canceled) {
		return domain.JobCanceled, string(domain.CodeCanceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return domain.JobTimedOut, string(domain.CodeTimeout)
	}
	switch domain.ErrorCodeOf(err) {
	case domain.CodeNonConverged:
		return domain.JobFailed, string(domain.CodeNonConverged)
	case domain.CodeUnavailable:
		return domain.JobFailed, string(domain.CodeRetryExhausted)
	default:
		return domain.JobFailed, "execution_error"
	}
}

func snapshotArcsForSolution(st *persistence.State, targetID string) []domain.ObservationArc {
	return persistence.ArcsForTarget(st, targetID)
}
