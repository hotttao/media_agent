package worker_gateway

const (
	FailureTransient = "transient"
	FailurePermanent = "permanent"
)

// Classification only describes the task outcome. Conductor alone decides
// whether, when and how many times to execute a new technical attempt.
func failureStatus(kind string) (string, error) {
	switch kind {
	case FailureTransient:
		return "FAILED", nil
	case "", FailurePermanent:
		return "FAILED_WITH_TERMINAL_ERROR", nil
	default:
		return "", ErrInvalidRequest
	}
}

func failureResult(reason, kind string) (map[string]any, error) {
	if reason == "" {
		return nil, ErrInvalidRequest
	}
	if _, err := failureStatus(kind); err != nil {
		return nil, err
	}
	result := map[string]any{"reason": reason}
	// Keep legacy JSON unchanged so old receipts remain replayable.
	if kind != "" && kind != FailurePermanent {
		result["failure_kind"] = kind
	}
	return result, nil
}
