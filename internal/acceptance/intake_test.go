package acceptance

import (
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/intake"
	"sync"
	"testing"
)

func TestDV01ValidArcFormsClosedLoop(t *testing.T) {
	app := newTestApp(t, "normal")
	arc := submitAndAssociate(t, app, publicArc("STA-ALPHA", "PUB-0001", 0, 121.1))
	if arc.AssociatedTargetID == "" {
		t.Fatalf("expected associated target")
	}
	target, arcs, _, _, err := app.Association.Target(arc.AssociatedTargetID)
	if err != nil {
		t.Fatalf("target query: %v", err)
	}
	if target.AssociationRevision != 1 || len(arcs) != 1 {
		t.Fatalf("unexpected target closure: revision=%d arcs=%d", target.AssociationRevision, len(arcs))
	}
}

func TestDV02InvalidInputsStopAtIntakeBoundary(t *testing.T) {
	app := newTestApp(t, "normal")
	cases := []intake.SubmitArcRequest{
		{StationID: "", ArcID: "MISS", Confidence: 0.9, Samples: publicArc("STA-ALPHA", "X", 0, 10).Samples},
		{StationID: "STA-ALPHA", ArcID: "BAD-AZ", Confidence: 0.9, Samples: []intake.SampleRequest{{Time: "2026-08-25T00:00:00Z", AzimuthDeg: 360, ElevationDeg: 0}, {Time: "2026-08-25T00:00:01Z", AzimuthDeg: 1, ElevationDeg: 0}, {Time: "2026-08-25T00:00:02Z", AzimuthDeg: 2, ElevationDeg: 0}}},
		{StationID: "STA-ALPHA", ArcID: "BAD-EL", Confidence: 0.9, Samples: []intake.SampleRequest{{Time: "2026-08-25T00:00:00Z", AzimuthDeg: 1, ElevationDeg: -91}, {Time: "2026-08-25T00:00:01Z", AzimuthDeg: 1, ElevationDeg: 0}, {Time: "2026-08-25T00:00:02Z", AzimuthDeg: 2, ElevationDeg: 0}}},
		{StationID: "STA-ALPHA", ArcID: "BAD-CONF", Confidence: 1.2, Samples: publicArc("STA-ALPHA", "X", 0, 10).Samples},
		{StationID: "STA-ALPHA", ArcID: "BAD-TIME", Confidence: 0.9, Samples: []intake.SampleRequest{{Time: "2026-08-25T00:00:00", AzimuthDeg: 1, ElevationDeg: 0}, {Time: "2026-08-25T00:00:01Z", AzimuthDeg: 1, ElevationDeg: 0}, {Time: "2026-08-25T00:00:02Z", AzimuthDeg: 2, ElevationDeg: 0}}},
		{StationID: "STA-ALPHA", ArcID: "BAD-ORDER", Confidence: 0.9, Samples: []intake.SampleRequest{{Time: "2026-08-25T00:00:00Z", AzimuthDeg: 1, ElevationDeg: 0}, {Time: "2026-08-25T00:00:00Z", AzimuthDeg: 1, ElevationDeg: 0}, {Time: "2026-08-25T00:00:02Z", AzimuthDeg: 2, ElevationDeg: 0}}},
	}
	for i, req := range cases {
		if _, err := app.Intake.SubmitArc(req); !domain.IsCode(err, domain.CodeValidation) {
			t.Fatalf("case %d expected validation error, got %v", i, err)
		}
	}
	state, err := app.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.ObservationArcs) != 0 {
		t.Fatalf("invalid arcs should not persist, got %d", len(state.ObservationArcs))
	}
}

func TestDV03ConcurrentDuplicateSubmissionCreatesOneChain(t *testing.T) {
	app := newTestApp(t, "normal")
	req := publicArc("STA-ALPHA", "DUP-0001", 0, 122.0)
	var wg sync.WaitGroup
	errs := make(chan error, 25)
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := app.Intake.SubmitArc(req)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("duplicate submit failed: %v", err)
		}
	}
	if err := app.Association.ProcessPending(); err != nil {
		t.Fatal(err)
	}
	state, err := app.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.ObservationArcs) != 1 || len(state.Targets) != 1 || len(state.Events) != 2 {
		t.Fatalf("duplicate chain was not collapsed: arcs=%d targets=%d events=%d", len(state.ObservationArcs), len(state.Targets), len(state.Events))
	}
}

func TestDV04ConflictingDuplicatePreservesOriginalPayload(t *testing.T) {
	app := newTestApp(t, "normal")
	req := publicArc("STA-ALPHA", "CONFLICT-1", 0, 122.0)
	first, err := app.Intake.SubmitArc(req)
	if err != nil {
		t.Fatal(err)
	}
	req.Samples[1].AzimuthDeg += 3
	if _, err := app.Intake.SubmitArc(req); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	arc, err := app.Intake.GetArc(first.ArcKey)
	if err != nil {
		t.Fatal(err)
	}
	if arc.PayloadHash != first.PayloadHash {
		t.Fatalf("original payload hash changed")
	}
}
