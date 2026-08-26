package intake

type SampleRequest struct {
	Time         string  `json:"time"`
	AzimuthDeg   float64 `json:"azimuth_deg"`
	ElevationDeg float64 `json:"elevation_deg"`
}

type SubmitArcRequest struct {
	StationID  string          `json:"station_id"`
	ArcID      string          `json:"arc_id"`
	Confidence float64         `json:"confidence"`
	Samples    []SampleRequest `json:"samples"`
}

type SubmitArcResult struct {
	Accepted           bool   `json:"accepted"`
	Duplicate          bool   `json:"duplicate"`
	ArcKey             string `json:"arc_key"`
	PayloadHash        string `json:"payload_hash"`
	ReceivedSeq        int64  `json:"received_seq"`
	Quarantined        bool   `json:"quarantined"`
	QuarantineReason   string `json:"quarantine_reason,omitempty"`
	AssociatedTargetID string `json:"associated_target_id,omitempty"`
}
