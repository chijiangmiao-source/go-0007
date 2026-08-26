package orbit

import (
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/persistence"
)

type CancelResult struct {
	JobID       string `json:"job_id"`
	Applied     bool   `json:"applied"`
	FinalStatus string `json:"final_status"`
}

func (s *Scheduler) Cancel(jobID, reason string) (CancelResult, error) {
	if reason == "" {
		reason = "operator requested cancellation"
	}
	var out CancelResult
	err := s.store.Update(func(st *persistence.State) error {
		job, ok := st.SolveJobs[jobID]
		if !ok {
			return domain.Errorf(domain.CodeNotFound, "job %s not found", jobID)
		}
		out.JobID = jobID
		if domain.IsTerminalJob(job.Status) {
			out.FinalStatus = string(job.Status)
			out.Applied = false
			return nil
		}
		if !domain.CanTransitionJob(job.Status, domain.JobCanceled) {
			return (&domain.AppError{Code: domain.CodeIllegalState, Message: "cannot cancel from current state", State: string(job.Status)})
		}
		job.Status = domain.JobCanceled
		job.CancelReason = reason
		ev := persistence.AppendEvent(st, persistence.EventSolveCanceled, job.ID, job.InputSnapshotHash, reason)
		job.CancelEventSeq = ev.Seq
		job.UpdatedAt = ev.RecordedAt
		st.SolveJobs[job.ID] = job
		out.Applied = true
		out.FinalStatus = string(job.Status)
		return nil
	})
	return out, err
}

func (s *Scheduler) GetJob(jobID string) (domain.SolveJob, error) {
	var job domain.SolveJob
	err := s.store.View(func(st *persistence.State) error {
		found, ok := st.SolveJobs[jobID]
		if !ok {
			return domain.Errorf(domain.CodeNotFound, "job %s not found", jobID)
		}
		job = found
		return nil
	})
	return job, err
}
