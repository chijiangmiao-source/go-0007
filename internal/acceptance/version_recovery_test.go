package acceptance

import (
	"bytes"
	"encoding/json"
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/intake"
	"leo-debris-orbit-loop/internal/versioning"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestDV13ConcurrentFreezeCreatesSingleImmutableVersion(t *testing.T) {
	app := newTestApp(t, "normal")
	arc := submitAndAssociate(t, app, publicArc("STA-ALPHA", "FREEZE-1", 0, 121))
	target, _, _, _, _ := app.Association.Target(arc.AssociatedTargetID)
	job, review := solveAndReview(t, app, target.ID, target.AssociationRevision)
	if review.Status != domain.ReviewApproved {
		t.Fatalf("solution should be approved before freeze")
	}
	var wg sync.WaitGroup
	results := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := app.Versioning.Freeze(target.ID, versioning.FreezeRequest{SolutionID: job.ResultSolutionID, ExpectedCurrentVersion: 0})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected one freeze success, got %d", successes)
	}
	versions, err := app.Versioning.List(target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Version != 1 || versions[0].ContentHash == "" {
		t.Fatalf("bad versions: %+v", versions)
	}
	if _, err := app.Versioning.Freeze(target.ID, versioning.FreezeRequest{SolutionID: job.ResultSolutionID, ExpectedCurrentVersion: 1}); !domain.IsCode(err, domain.CodeIllegalState) && !domain.IsCode(err, domain.CodeFrozenImmutable) {
		t.Fatalf("expected immutable freeze rejection, got %v", err)
	}
}

func TestPublicHTTPObservationEndpointAndStateAreUsable(t *testing.T) {
	app := newTestApp(t, "normal")
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	body, _ := json.Marshal(publicArc("STA-ALPHA", "HTTP-1", 0, 121))
	resp, err := http.Post(server.URL+"/v1/observation-arcs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
	var accepted struct {
		ArcKey             string `json:"arc_key"`
		AssociatedTargetID string `json:"associated_target_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.ArcKey == "" || accepted.AssociatedTargetID == "" {
		t.Fatalf("incomplete api response: %+v", accepted)
	}
	stateResp, err := http.Get(server.URL + "/v1/system/state")
	if err != nil {
		t.Fatal(err)
	}
	defer stateResp.Body.Close()
	var state map[string]any
	if err := json.NewDecoder(stateResp.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if state["arcs"].(float64) != 1 || state["targets"].(float64) != 1 {
		t.Fatalf("state endpoint did not expose live backend state: %+v", state)
	}
}

func TestPublicDemoFixturesUseRegisteredStations(t *testing.T) {
	req := publicArc("STA-BRAVO", "FIXTURE-1", 2, 119)
	if req.StationID != "STA-BRAVO" || len(req.Samples) != 3 {
		t.Fatalf("fixture malformed: %+v", req)
	}
	_ = intake.SubmitArcRequest(req)
}
