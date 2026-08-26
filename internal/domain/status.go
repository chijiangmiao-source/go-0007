package domain

func CanTransitionJob(from, to SolveJobStatus) bool {
	switch from {
	case JobQueued:
		return to == JobRunning || to == JobCanceled
	case JobRunning:
		return to == JobSucceeded || to == JobFailed || to == JobTimedOut || to == JobCanceled
	default:
		return false
	}
}

func IsTerminalJob(s SolveJobStatus) bool {
	return s == JobSucceeded || s == JobFailed || s == JobTimedOut || s == JobCanceled
}

func CanReviewDecision(status ReviewStatus) bool {
	return status == ReviewPending
}

func CanFreezeReview(status ReviewStatus) bool {
	return status == ReviewApproved
}
