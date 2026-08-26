package acceptance

import (
	"leo-debris-orbit-loop/internal/domain"
	"testing"
)

func TestDV05CrossMidnightOutOfOrderSamplesSortByAbsoluteTime(t *testing.T) {
	app := newTestApp(t, "normal")
	req := publicArc("STA-ALPHA", "MIDNIGHT-1", 0, 130)
	req.Samples[0], req.Samples[2] = req.Samples[2], req.Samples[0]
	arc := submitAndAssociate(t, app, req)
	for i := 1; i < len(arc.CanonicalSamples); i++ {
		if !arc.CanonicalSamples[i].ObservedAt.After(arc.CanonicalSamples[i-1].ObservedAt) {
			t.Fatalf("samples are not sorted by absolute time")
		}
	}
}

func TestDV06StableTieBreakerDoesNotDependOnMapOrder(t *testing.T) {
	var chosen string
	for i := 0; i < 100; i++ {
		app := newTestApp(t, "normal")
		a := submitAndAssociate(t, app, publicArc("STA-ALPHA", "TIE-A", 0, 10))
		submitAndAssociate(t, app, publicArc("STA-BRAVO", "TIE-B", 0, 250))
		bridge := submitAndAssociate(t, app, publicArc("STA-ALPHA", "TIE-C", 0, 130))
		if i == 0 {
			chosen = bridge.AssociatedTargetID
		}
		if bridge.AssociatedTargetID != chosen {
			t.Fatalf("unstable choice: first=%s now=%s alpha=%s", chosen, bridge.AssociatedTargetID, a.AssociatedTargetID)
		}
	}
}

func TestDV07ClockDriftIsolatesOnlyAffectedStation(t *testing.T) {
	app := newTestApp(t, "normal")
	drift := submitAndAssociate(t, app, publicArc("STA-DRIFT", "DRIFT-1", 0, 125))
	good := submitAndAssociate(t, app, publicArc("STA-ALPHA", "GOOD-1", 0, 125))
	if !drift.Quarantined || drift.AssociatedTargetID != "" {
		t.Fatalf("drifted station should be quarantined without association")
	}
	if good.Quarantined || good.AssociatedTargetID == "" {
		t.Fatalf("healthy station should continue through association")
	}
}

func TestDV14RecoveryValidatesEventChainAndCancelsUnfinishedJobs(t *testing.T) {
	app := newTestApp(t, "normal")
	arc := submitAndAssociate(t, app, publicArc("STA-ALPHA", "REC-1", 0, 121))
	target, _, _, _, err := app.Association.Target(arc.AssociatedTargetID)
	if err != nil {
		t.Fatal(err)
	}
	job, err := app.Scheduler.CreateJob(target.ID, target.AssociationRevision)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Recovery.Recover(); err != nil {
		t.Fatalf("recover: %v", err)
	}
	got, err := app.Scheduler.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.JobCanceled {
		t.Fatalf("unfinished job should be canceled on recovery, got %s", got.Status)
	}
}
