package acceptance

import (
	"encoding/json"
	"leo-debris-orbit-loop/internal/api"
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/orbit"
	"leo-debris-orbit-loop/internal/persistence"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestModel_EventRecordedAtIntegrity(t *testing.T) {
	cases := []struct {
		name      string
		tamperSeq int64
	}{
		{name: "observation received timestamp is authenticated", tamperSeq: 1},
		{name: "target associated timestamp is authenticated", tamperSeq: 2},
		{name: "intact chain retains recovery and event pagination", tamperSeq: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storePath := filepath.Join(t.TempDir(), "state.json")
			app := api.NewApp(storePath, orbit.NewDeterministicEngine("normal"))
			if err := app.Recovery.Recover(); err != nil {
				t.Fatalf("initialize store: %v", err)
			}
			arc := submitAndAssociate(t, app, publicArc("STA-ALPHA", "RECORDED-AT-1", 0, 121))

			if tc.tamperSeq != 0 {
				state, err := app.Store.Load()
				if err != nil {
					t.Fatalf("load state for tampering: %v", err)
				}
				if len(state.Events) != 2 {
					t.Fatalf("expected received and associated events, got %+v", state.Events)
				}
				idx := int(tc.tamperSeq - 1)
				state.Events[idx].RecordedAt = state.Events[idx].RecordedAt.Add(24 * time.Hour)
				contents, err := json.MarshalIndent(state, "", "  ")
				if err != nil {
					t.Fatalf("marshal tampered state: %v", err)
				}
				if err := os.WriteFile(storePath, contents, 0o644); err != nil {
					t.Fatalf("persist tampered state: %v", err)
				}

				restarted := api.NewApp(storePath, orbit.NewDeterministicEngine("normal"))
				err = restarted.Recovery.Recover()
				if !domain.IsCode(err, domain.CodeCorruptStore) {
					t.Fatalf("recovery after changing recorded_at at seq %d returned %v, want corrupt_store", tc.tamperSeq, err)
				}
				return
			}

			target, _, _, _, err := app.Association.Target(arc.AssociatedTargetID)
			if err != nil {
				t.Fatalf("load associated target: %v", err)
			}
			job, err := app.Scheduler.CreateJob(target.ID, target.AssociationRevision)
			if err != nil {
				t.Fatalf("create queued job: %v", err)
			}

			restarted := api.NewApp(storePath, orbit.NewDeterministicEngine("normal"))
			if err := restarted.Recovery.Recover(); err != nil {
				t.Fatalf("recover intact state: %v", err)
			}
			recoveredJob, err := restarted.Scheduler.GetJob(job.ID)
			if err != nil {
				t.Fatalf("load recovered job: %v", err)
			}
			if recoveredJob.Status != domain.JobCanceled || recoveredJob.CancelEventSeq != 4 {
				t.Fatalf("unfinished job was not canceled by recovery: %+v", recoveredJob)
			}

			query := func(after int64, limit int) persistence.EventPage {
				t.Helper()
				req := httptest.NewRequest(http.MethodGet, "/v1/system/events?after_seq="+
					strconv.FormatInt(after, 10)+"&limit="+strconv.Itoa(limit), nil)
				resp := httptest.NewRecorder()
				restarted.Handler().ServeHTTP(resp, req)
				if resp.Code != http.StatusOK {
					t.Fatalf("events response status = %d, body = %s", resp.Code, resp.Body.String())
				}
				var page persistence.EventPage
				if err := json.Unmarshal(resp.Body.Bytes(), &page); err != nil {
					t.Fatalf("decode events page: %v", err)
				}
				return page
			}

			first := query(1, 2)
			if first.NextSeq != 3 || len(first.Events) != 2 ||
				first.Events[0].Seq != 2 || first.Events[0].Type != persistence.EventTargetAssociated ||
				first.Events[1].Seq != 3 || first.Events[1].Type != persistence.EventSolveQueued {
				t.Fatalf("unexpected first event page: %+v", first)
			}
			second := query(first.NextSeq, 2)
			if second.NextSeq != 4 || len(second.Events) != 1 ||
				second.Events[0].Seq != 4 || second.Events[0].Type != persistence.EventSolveCanceled ||
				second.Events[0].CausationID != "recovery" {
				t.Fatalf("unexpected second event page: %+v", second)
			}
		})
	}
}
