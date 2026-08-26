package acceptance

import (
	"bytes"
	"leo-debris-orbit-loop/internal/api"
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/orbit"
	"leo-debris-orbit-loop/internal/persistence"
	"os"
	"testing"
)

func TestModel_RecoveryRejectsCursorAnomaliesWithoutCorruptingEventLog(t *testing.T) {
	tests := []struct {
		name       string
		initial    domain.SolveJobStatus
		eventCount int
		cursor     int64
		wantReject bool
	}{
		{name: "cursor behind log tail", initial: domain.JobQueued, eventCount: 2, cursor: 2, wantReject: true},
		{name: "cursor ahead of log tail", initial: domain.JobRunning, eventCount: 2, cursor: 4, wantReject: true},
		{name: "noninitial cursor on empty log", initial: domain.JobQueued, eventCount: 0, cursor: 2, wantReject: true},
		{name: "consistent queued job is canceled once", initial: domain.JobQueued, eventCount: 1, cursor: 2},
		{name: "consistent running job is canceled once", initial: domain.JobRunning, eventCount: 2, cursor: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := t.TempDir() + "/state.json"
			app := api.NewApp(path, orbit.NewDeterministicEngine("normal"))
			if err := app.Store.Update(func(st *persistence.State) error {
				for i := 0; i < tt.eventCount; i++ {
					persistence.AppendEvent(st, persistence.EventSolveQueued, "JOB-000001", "snapshot", "seed")
				}
				st.SolveJobs["JOB-000001"] = domain.SolveJob{
					ID: "JOB-000001", Status: tt.initial, InputSnapshotHash: "snapshot",
				}
				st.NextEvent = tt.cursor
				return nil
			}); err != nil {
				t.Fatalf("seed store: %v", err)
			}

			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read seeded store: %v", err)
			}
			err = app.Recovery.Recover()
			if tt.wantReject {
				if !domain.IsCode(err, domain.CodeCorruptStore) {
					t.Fatalf("Recover() error = %v, want corrupt_store", err)
				}
				after, readErr := os.ReadFile(path)
				if readErr != nil {
					t.Fatalf("read rejected store: %v", readErr)
				}
				if !bytes.Equal(after, before) {
					t.Fatal("rejected recovery rewrote persistent state")
				}
				return
			}

			if err != nil {
				t.Fatalf("Recover() error = %v", err)
			}
			state, err := app.Store.Load()
			if err != nil {
				t.Fatalf("load recovered store: %v", err)
			}
			job := state.SolveJobs["JOB-000001"]
			wantCancelSeq := int64(tt.eventCount + 1)
			if job.Status != domain.JobCanceled || job.CancelEventSeq != wantCancelSeq {
				t.Fatalf("recovered job = %+v, want canceled at event %d", job, wantCancelSeq)
			}
			if len(state.Events) != tt.eventCount+1 || state.Events[tt.eventCount].Seq != wantCancelSeq || state.Events[tt.eventCount].Type != persistence.EventSolveCanceled {
				t.Fatalf("unexpected recovered events: %+v", state.Events)
			}
			if state.NextEvent != wantCancelSeq+1 || state.Checkpoint.LastAppliedSeq != wantCancelSeq {
				t.Fatalf("cursor/checkpoint after recovery = %d/%d, want %d/%d", state.NextEvent, state.Checkpoint.LastAppliedSeq, wantCancelSeq+1, wantCancelSeq)
			}
			if err := persistence.ValidateEventChain(state.Events); err != nil {
				t.Fatalf("recovery produced invalid event chain: %v", err)
			}
			if err := app.Recovery.Recover(); err != nil {
				t.Fatalf("second Recover() error = %v", err)
			}
			again, err := app.Store.Load()
			if err != nil {
				t.Fatalf("load second recovery: %v", err)
			}
			if len(again.Events) != len(state.Events) || again.NextEvent != state.NextEvent {
				t.Fatalf("second recovery appended another cancellation: events/cursor %d/%d, want %d/%d", len(again.Events), again.NextEvent, len(state.Events), state.NextEvent)
			}
			if err := persistence.ValidateEventChain(again.Events); err != nil {
				t.Fatalf("second recovery could not validate event chain: %v", err)
			}
		})
	}
}
