package monitoring

import "time"

type (
	Service interface {
		Status() ([]Status, error)
	}

	Status struct {
		Module    string
		Timestamp time.Time
		State     State
		// Human-friendly description of the current module state
		Message string
	}
)

type State string

const (
	State_Running State = "running"
	State_Failed        = "failed"
)
