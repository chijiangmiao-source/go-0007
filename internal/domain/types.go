package domain

import "time"

type Station struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	LatitudeDeg         float64   `json:"latitude_deg"`
	LongitudeDeg        float64   `json:"longitude_deg"`
	AltitudeM           float64   `json:"altitude_m"`
	Enabled             bool      `json:"enabled"`
	AllowedClockDriftMS int64     `json:"allowed_clock_drift_ms"`
	CurrentDriftMS      int64     `json:"current_drift_ms"`
	ConfigRevision      int64     `json:"config_revision"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type AngleSample struct {
	RawTime      string    `json:"raw_time"`
	ObservedAt   time.Time `json:"observed_at"`
	AzimuthDeg   float64   `json:"azimuth_deg"`
	ElevationDeg float64   `json:"elevation_deg"`
}

type ObservationArc struct {
	ID                 string        `json:"id"`
	StationID          string        `json:"station_id"`
	ArcID              string        `json:"arc_id"`
	OriginalSamples    []AngleSample `json:"original_samples"`
	CanonicalSamples   []AngleSample `json:"canonical_samples"`
	Confidence         float64       `json:"confidence"`
	PayloadHash        string        `json:"payload_hash"`
	ReceivedSeq        int64         `json:"received_seq"`
	ReceivedAt         time.Time     `json:"received_at"`
	Quarantined        bool          `json:"quarantined"`
	QuarantineReason   string        `json:"quarantine_reason,omitempty"`
	AssociatedTargetID string        `json:"associated_target_id,omitempty"`
	AssociationEvent   int64         `json:"association_event,omitempty"`
}

type CatalogTarget struct {
	ID                   string    `json:"id"`
	CreatedEventSeq      int64     `json:"created_event_seq"`
	AssociationRevision  int64     `json:"association_revision"`
	LifecycleState       string    `json:"lifecycle_state"`
	CurrentFrozenVersion int       `json:"current_frozen_version"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type AssociationScore struct {
	TimeDeltaSeconds float64 `json:"time_delta_seconds"`
	AngleResidualDeg float64 `json:"angle_residual_deg"`
	GeometryPenalty  float64 `json:"geometry_penalty"`
	ConfidenceBias   float64 `json:"confidence_bias"`
	Total            float64 `json:"total"`
}

type AssociationDecision struct {
	ID                  string                      `json:"id"`
	ArcKey              string                      `json:"arc_key"`
	CandidateScores     map[string]AssociationScore `json:"candidate_scores"`
	TieBreaker          string                      `json:"tie_breaker"`
	FinalTargetID       string                      `json:"final_target_id"`
	ConfigRevision      int64                       `json:"config_revision"`
	DecisionEventSeq    int64                       `json:"decision_event_seq"`
	AssociationRevision int64                       `json:"association_revision"`
	DecidedAt           time.Time                   `json:"decided_at"`
}

type SolveJobStatus string

const (
	JobQueued    SolveJobStatus = "queued"
	JobRunning   SolveJobStatus = "running"
	JobSucceeded SolveJobStatus = "succeeded"
	JobFailed    SolveJobStatus = "failed"
	JobTimedOut  SolveJobStatus = "timed_out"
	JobCanceled  SolveJobStatus = "canceled"
)

type SolveJob struct {
	ID                  string         `json:"id"`
	TargetID            string         `json:"target_id"`
	AssociationRevision int64          `json:"association_revision"`
	InputSnapshotHash   string         `json:"input_snapshot_hash"`
	CalculatorVersion   string         `json:"calculator_version"`
	Status              SolveJobStatus `json:"status"`
	Attempts            int            `json:"attempts"`
	Deadline            time.Time      `json:"deadline"`
	FailureClass        string         `json:"failure_class,omitempty"`
	CancelReason        string         `json:"cancel_reason,omitempty"`
	CancelEventSeq      int64          `json:"cancel_event_seq,omitempty"`
	ResultSolutionID    string         `json:"result_solution_id,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

type OrbitParameters struct {
	SemiMajorAxisKM float64 `json:"semi_major_axis_km"`
	Eccentricity    float64 `json:"eccentricity"`
	InclinationDeg  float64 `json:"inclination_deg"`
	RAANDeg         float64 `json:"raan_deg"`
	ArgPerigeeDeg   float64 `json:"arg_perigee_deg"`
	MeanAnomalyDeg  float64 `json:"mean_anomaly_deg"`
}

type IterationSummary struct {
	Iterations int     `json:"iterations"`
	LastDelta  float64 `json:"last_delta"`
	Converged  bool    `json:"converged"`
}

type OrbitSolution struct {
	ID                string           `json:"id"`
	JobID             string           `json:"job_id"`
	TargetID          string           `json:"target_id"`
	Epoch             time.Time        `json:"epoch"`
	Parameters        OrbitParameters  `json:"parameters"`
	Iteration         IterationSummary `json:"iteration"`
	OutputHash        string           `json:"output_hash"`
	GeneratedAt       time.Time        `json:"generated_at"`
	ObservationArcIDs []string         `json:"observation_arc_ids"`
	Residuals         []ResidualPoint  `json:"residuals,omitempty"`
}

type ResidualPoint struct {
	ArcID          string    `json:"arc_id"`
	SampleTime     time.Time `json:"sample_time"`
	AzimuthError   float64   `json:"azimuth_error"`
	ElevationError float64   `json:"elevation_error"`
	MagnitudeDeg   float64   `json:"magnitude_deg"`
}

type ResidualSummary struct {
	RMSDeg      float64 `json:"rms_deg"`
	MaxDeg      float64 `json:"max_deg"`
	SampleCount int     `json:"sample_count"`
}

type ThresholdSnapshot struct {
	PublishRMSDeg  float64 `json:"publish_rms_deg"`
	RejectRMSDeg   float64 `json:"reject_rms_deg"`
	ConfigRevision int64   `json:"config_revision"`
}

type ReviewStatus string

const (
	ReviewApproved ReviewStatus = "approved"
	ReviewPending  ReviewStatus = "pending_review"
	ReviewRejected ReviewStatus = "rejected"
	ReviewFrozen   ReviewStatus = "frozen"
)

type ResidualReview struct {
	SolutionID       string            `json:"solution_id"`
	Residuals        []ResidualPoint   `json:"residuals"`
	Summary          ResidualSummary   `json:"summary"`
	Thresholds       ThresholdSnapshot `json:"thresholds"`
	Status           ReviewStatus      `json:"status"`
	Decision         string            `json:"decision,omitempty"`
	Reason           string            `json:"reason,omitempty"`
	DecisionEventSeq int64             `json:"decision_event_seq,omitempty"`
	ReviewedAt       time.Time         `json:"reviewed_at,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
}

type FrozenVersion struct {
	TargetID        string          `json:"target_id"`
	Version         int             `json:"version"`
	SolutionID      string          `json:"solution_id"`
	FreezeEventSeq  int64           `json:"freeze_event_seq"`
	ContentHash     string          `json:"content_hash"`
	PublishedAt     time.Time       `json:"published_at"`
	Orbit           OrbitParameters `json:"orbit"`
	InputArcIDs     []string        `json:"input_arc_ids"`
	ResidualSummary ResidualSummary `json:"residual_summary"`
	ResultHash      string          `json:"result_hash"`
}

type EventRecord struct {
	Seq         int64     `json:"seq"`
	Type        string    `json:"type"`
	AggregateID string    `json:"aggregate_id"`
	PayloadHash string    `json:"payload_hash"`
	CausationID string    `json:"causation_id,omitempty"`
	Checksum    string    `json:"checksum"`
	RecordedAt  time.Time `json:"recorded_at"`
}

type Checkpoint struct {
	LastAppliedSeq int64     `json:"last_applied_seq"`
	StateHash      string    `json:"state_hash"`
	UpdatedAt      time.Time `json:"updated_at"`
}
