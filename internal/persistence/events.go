package persistence

import (
	"fmt"
	"leo-debris-orbit-loop/internal/domain"
	"time"
)

// eventChecksumFields is the canonical tuple authenticated by each event's
// checksum. RecordedAt is included so that tampering with the persisted
// timestamp is detected as a checksum mismatch during recovery. The timestamp
// is rendered with RFC3339Nano because that is the form JSON round-trips
// through, keeping the checksum stable across persist/reload and independent
// of any monotonic clock reading.
const eventChecksumFormat = "%d|%s|%s|%s|%s|%s|%s"

func eventChecksum(ev domain.EventRecord, prev string) string {
	return domain.HashBytes([]byte(fmt.Sprintf(eventChecksumFormat,
		ev.Seq, ev.Type, ev.AggregateID, ev.PayloadHash, ev.CausationID,
		ev.RecordedAt.UTC().Format(time.RFC3339Nano), prev)))
}

const (
	EventObservationReceived = "observation.received"
	EventObservationRejected = "observation.rejected"
	EventTargetAssociated    = "target.associated"
	EventSolveQueued         = "solve.queued"
	EventSolveRunning        = "solve.running"
	EventSolveSucceeded      = "solve.succeeded"
	EventSolveFailed         = "solve.failed"
	EventSolveCanceled       = "solve.canceled"
	EventResidualReviewed    = "residual.reviewed"
	EventVersionFrozen       = "version.frozen"
)

func AppendEvent(st *State, typ, aggregate, payloadHash, causation string) domain.EventRecord {
	prev := ""
	if len(st.Events) > 0 {
		prev = st.Events[len(st.Events)-1].Checksum
	}
	seq := st.NextEvent
	ev := domain.EventRecord{
		Seq: seq, Type: typ, AggregateID: aggregate, PayloadHash: payloadHash,
		CausationID: causation, RecordedAt: domain.NowUTC(),
	}
	ev.Checksum = eventChecksum(ev, prev)
	st.Events = append(st.Events, ev)
	st.NextEvent++
	return ev
}

func ValidateEventChain(events []domain.EventRecord) error {
	prev := ""
	for i, ev := range events {
		expectedSeq := int64(i + 1)
		if ev.Seq != expectedSeq {
			return domain.Errorf(domain.CodeCorruptStore, "event sequence gap at %d", expectedSeq)
		}
		sum := eventChecksum(ev, prev)
		if ev.Checksum != sum {
			return domain.Errorf(domain.CodeCorruptStore, "event checksum mismatch at seq %d", ev.Seq)
		}
		prev = ev.Checksum
	}
	return nil
}
