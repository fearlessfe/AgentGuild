package domain

import "time"

const (
	LeaseDuration = 10 * time.Minute
	LeaseGrace    = 30 * time.Second
)

type ExecutionStatus string

const (
	ExecutionLeased            ExecutionStatus = "leased"
	ExecutionRunning           ExecutionStatus = "running"
	ExecutionSubmitted         ExecutionStatus = "submitted"
	ExecutionValidating        ExecutionStatus = "validating"
	ExecutionReviewing         ExecutionStatus = "reviewing"
	ExecutionRevisionRequested ExecutionStatus = "revision_requested"
	ExecutionAccepted          ExecutionStatus = "accepted"
	ExecutionRejected          ExecutionStatus = "rejected"
	ExecutionExpired           ExecutionStatus = "expired"
	ExecutionCancelled         ExecutionStatus = "cancelled"
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
	Stage    string
	Progress float64
	Lease    Lease
	ClaimedAt       time.Time
	StartedAt       time.Time
	SubmittedAt     time.Time
	ExpiredAt       time.Time
	LastHeartbeatAt time.Time
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
) (*Execution, error) {
	if id == "" {
		return nil, invalidArgument("id")
	}
	if taskID == "" {
		return nil, invalidArgument("task_id")
	}
	if tenantID == "" {
		return nil, invalidArgument("tenant_id")
	}
	if agentID == "" {
		return nil, invalidArgument("agent_id")
	}
	if generation < 0 {
		return nil, invalidArgument("generation")
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
	}, nil
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
	case IntentCancel:
		return e.Cancel(actor, now)
	default:
		return ErrStateConflict
	}
}

func (e *Execution) Cancel(actor Actor, _ time.Time) error {
	switch e.Status {
	case ExecutionLeased, ExecutionRunning, ExecutionSubmitted, ExecutionValidating,
		ExecutionReviewing, ExecutionRevisionRequested:
	default:
		return ErrStateConflict
	}
	if actor.ID == "" || (actor.Type != ActorPublisher && actor.Type != ActorSystem) {
		return ErrForbidden
	}
	e.Status = ExecutionCancelled
	return nil
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
