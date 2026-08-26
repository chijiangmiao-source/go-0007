package intake

import (
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/persistence"
)

type Service struct {
	store *persistence.Store
}

func New(store *persistence.Store) *Service {
	return &Service{store: store}
}

func (s *Service) SubmitArc(req SubmitArcRequest) (SubmitArcResult, error) {
	samples, payloadHash, err := normalize(req)
	if err != nil {
		return SubmitArcResult{}, err
	}
	var result SubmitArcResult
	err = s.store.Update(func(st *persistence.State) error {
		station, ok := st.Stations[req.StationID]
		if !ok {
			return domain.Errorf(domain.CodeValidation, "station %s is not registered", req.StationID)
		}
		key := persistence.ArcKey(req.StationID, req.ArcID)
		if existingID, ok := st.ArcIndex[key]; ok {
			existing := st.ObservationArcs[existingID]
			if existing.PayloadHash != payloadHash {
				return domain.Errorf(domain.CodeConflict, "arc %s was already accepted with a different payload", key)
			}
			result = SubmitArcResult{Accepted: true, Duplicate: true, ArcKey: key, PayloadHash: existing.PayloadHash, ReceivedSeq: existing.ReceivedSeq, Quarantined: existing.Quarantined, QuarantineReason: existing.QuarantineReason, AssociatedTargetID: existing.AssociatedTargetID}
			return nil
		}
		arc := domain.ObservationArc{
			ID: key, StationID: req.StationID, ArcID: req.ArcID,
			OriginalSamples:  append([]domain.AngleSample(nil), samples...),
			CanonicalSamples: samples, Confidence: req.Confidence, PayloadHash: payloadHash,
			ReceivedAt: domain.NowUTC(),
		}
		if !station.Enabled {
			arc.Quarantined = true
			arc.QuarantineReason = "station disabled"
		} else if !StationAcceptsAssociation(station) {
			arc.Quarantined = true
			arc.QuarantineReason = "station clock drift exceeds configured allowance"
		}
		ev := persistence.AppendEvent(st, persistence.EventObservationReceived, arc.ID, payloadHash, "")
		arc.ReceivedSeq = ev.Seq
		st.ObservationArcs[arc.ID] = arc
		st.ArcIndex[key] = arc.ID
		result = SubmitArcResult{Accepted: true, ArcKey: key, PayloadHash: payloadHash, ReceivedSeq: arc.ReceivedSeq, Quarantined: arc.Quarantined, QuarantineReason: arc.QuarantineReason}
		return nil
	})
	return result, err
}

func (s *Service) GetArc(id string) (domain.ObservationArc, error) {
	var arc domain.ObservationArc
	err := s.store.View(func(st *persistence.State) error {
		found, ok := st.ObservationArcs[id]
		if !ok {
			return domain.Errorf(domain.CodeNotFound, "arc %s not found", id)
		}
		arc = found
		return nil
	})
	return arc, err
}
