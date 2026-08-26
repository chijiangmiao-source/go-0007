package acceptance

import (
	"bytes"
	"encoding/json"
	"leo-debris-orbit-loop/internal/api"
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/orbit"
	"leo-debris-orbit-loop/internal/persistence"
	"os"
	"path/filepath"
	"testing"
)

func TestModel_RecoveryRejectsCorruptEventChain(t *testing.T) {
	cases := []struct {
		name      string
		jobStatus domain.SolveJobStatus
		corrupt   func(*persistence.State)
	}{
		{
			name:      "event sequence gap with queued job",
			jobStatus: domain.JobQueued,
			corrupt: func(st *persistence.State) {
				st.Events[1].Seq++
			},
		},
		{
			name:      "broken forward checksum with running job",
			jobStatus: domain.JobRunning,
			corrupt: func(st *persistence.State) {
				st.Events[1].Checksum = "broken-checksum"
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storePath := filepath.Join(t.TempDir(), "state.json")
			store := persistence.NewStore(storePath)
			if err := store.Update(func(st *persistence.State) error {
				job := domain.SolveJob{
					ID:                "JOB-000001",
					InputSnapshotHash: "snapshot-hash",
					Status:            tc.jobStatus,
				}
				st.SolveJobs[job.ID] = job
				persistence.AppendEvent(st, persistence.EventSolveQueued, job.ID, job.InputSnapshotHash, "target")
				persistence.AppendEvent(st, persistence.EventSolveRunning, job.ID, job.InputSnapshotHash, "")
				return nil
			}); err != nil {
				t.Fatalf("prepare state: %v", err)
			}

			st, err := store.Load()
			if err != nil {
				t.Fatalf("load prepared state: %v", err)
			}
			tc.corrupt(st)
			corruptBytes, err := json.MarshalIndent(st, "", "  ")
			if err != nil {
				t.Fatalf("marshal corrupt state: %v", err)
			}
			if err := os.WriteFile(storePath, corruptBytes, 0o644); err != nil {
				t.Fatalf("write corrupt state: %v", err)
			}

			app := api.NewApp(storePath, orbit.NewDeterministicEngine("normal"))
			err = app.Recovery.Recover()
			if !domain.IsCode(err, domain.CodeCorruptStore) {
				t.Errorf("recovery error = %v, want code %s", err, domain.CodeCorruptStore)
			}

			after, readErr := os.ReadFile(storePath)
			if readErr != nil {
				t.Fatalf("read state after rejected recovery: %v", readErr)
			}
			if !bytes.Equal(after, corruptBytes) {
				t.Errorf("rejected recovery modified the corrupt state file")
			}
		})
	}
}
