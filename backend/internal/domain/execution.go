package domain

import "time"

const (
	LeaseDuration = 10 * time.Minute
	LeaseGrace    = 30 * time.Second
)

type ExecutionStatus string

const (
	ExecutionLeased   ExecutionStatus = "leased"
	ExecutionRunning  ExecutionStatus = "running"
	ExecutionAccepted ExecutionStatus = "accepted"
	ExecutionExpired  ExecutionStatus = "expired"
)

type Lease struct {
	Generation int64
	SoftExpiry time.Time
	HardExpiry time.Time
}

type Execution struct {
	ID       string
	TaskID   string
	TenantID string
	AgentID  string
	Status   ExecutionStatus
	Lease    Lease
}

func RenewLease(now time.Time, current int64) Lease {
	soft := now.Add(LeaseDuration)
	return Lease{
		Generation: current + 1,
		SoftExpiry: soft,
		HardExpiry: soft.Add(LeaseGrace),
	}
}

func NewLeasedExecution(
	id, taskID, tenantID, agentID string,
	now time.Time,
	generation int64,
) *Execution {
	if id == "" || taskID == "" || tenantID == "" || agentID == "" || generation < 0 {
		return nil
	}
	return &Execution{
		ID:       id,
		TaskID:   taskID,
		TenantID: tenantID,
		AgentID:  agentID,
		Status:   ExecutionLeased,
		Lease: Lease{
			Generation: generation,
			SoftExpiry: now.Add(LeaseDuration),
			HardExpiry: now.Add(LeaseDuration + LeaseGrace),
		},
	}
}

func (e *Execution) Start(now time.Time, generation int64) error {
	if e.Status != ExecutionLeased {
		return ErrStateConflict
	}
	if generation != e.Lease.Generation || !now.Before(e.Lease.HardExpiry) {
		return ErrLeaseExpired
	}
	e.Status = ExecutionRunning
	return nil
}

func (e *Execution) Heartbeat(now time.Time, generation int64) (Lease, error) {
	if e.Status != ExecutionLeased && e.Status != ExecutionRunning {
		return Lease{}, ErrStateConflict
	}
	if generation != e.Lease.Generation || !now.Before(e.Lease.HardExpiry) {
		return Lease{}, ErrLeaseExpired
	}
	e.Lease = RenewLease(now, generation)
	return e.Lease, nil
}

func (e *Execution) Expire(now time.Time) error {
	if e.Status != ExecutionLeased && e.Status != ExecutionRunning {
		return ErrStateConflict
	}
	if now.Before(e.Lease.HardExpiry) {
		return ErrStateConflict
	}
	e.Status = ExecutionExpired
	return nil
}

func (e *Execution) Apply(intent Intent, actor Actor, now time.Time) error {
	switch intent {
	case IntentAccept:
		return e.Accept(actor, now)
	default:
		return ErrStateConflict
	}
}

func (e *Execution) Accept(actor Actor, now time.Time) error {
	if e.Status != ExecutionRunning {
		return ErrStateConflict
	}
	if actor.Type != ActorReviewer || actor.ID == "" {
		return ErrForbidden
	}
	if !now.Before(e.Lease.HardExpiry) {
		return ErrLeaseExpired
	}
	e.Status = ExecutionAccepted
	return nil
}
