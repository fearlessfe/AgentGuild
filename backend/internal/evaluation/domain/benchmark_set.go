package domain

import (
	"time"
)

// BenchmarkSet is a versioned collection of tasks used to evaluate an Agent
// Version. Within a tenant the version_number monotonically increases.
type BenchmarkSet struct {
	id          string
	tenantID    string
	versionNumber int
	name        string
	description string
	isActive    bool
	createdBy   string
	createdAt   time.Time
	tasks       []BenchmarkTask
}

// BenchmarkTask is a lightweight reference to a task inside a BenchmarkSet.
type BenchmarkTask struct {
	TaskRef  string
	Ordering int
}

// NewBenchmarkSet creates a minimal BenchmarkSet. Version numbers are normally
// assigned by the repository.
func NewBenchmarkSet(tenantID, id, createdBy string, versionNumber int) (*BenchmarkSet, error) {
	if tenantID == "" {
		return nil, invalidArgument("tenant_id")
	}
	if id == "" {
		return nil, invalidArgument("id")
	}
	if createdBy == "" {
		return nil, invalidArgument("created_by")
	}
	if versionNumber < 1 {
		return nil, invalidArgument("version_number")
	}
	return &BenchmarkSet{
		id:            id,
		tenantID:      tenantID,
		versionNumber: versionNumber,
		createdBy:     createdBy,
		tasks:         []BenchmarkTask{},
	}, nil
}

// NewBenchmarkSetWithTasks creates a BenchmarkSet with an ordered task list.
func NewBenchmarkSetWithTasks(
	id, tenantID, createdBy string,
	versionNumber int,
	name, description string,
	tasks []BenchmarkTask,
	now time.Time,
) (*BenchmarkSet, error) {
	if now.IsZero() {
		return nil, invalidArgument("created_at")
	}
	bs, err := NewBenchmarkSet(tenantID, id, createdBy, versionNumber)
	if err != nil {
		return nil, err
	}
	bs.name = name
	bs.description = description
	bs.createdAt = now
	bs.tasks = append([]BenchmarkTask(nil), tasks...)
	return bs, nil
}

// ID returns the benchmark set identifier.
func (bs *BenchmarkSet) ID() string { return bs.id }

// TenantID returns the tenant identifier.
func (bs *BenchmarkSet) TenantID() string { return bs.tenantID }

// VersionNumber returns the tenant-local version number.
func (bs *BenchmarkSet) VersionNumber() int { return bs.versionNumber }

// Name returns the benchmark set name.
func (bs *BenchmarkSet) Name() string { return bs.name }

// Description returns the benchmark set description.
func (bs *BenchmarkSet) Description() string { return bs.description }

// IsActive reports whether this benchmark set is the current default.
func (bs *BenchmarkSet) IsActive() bool { return bs.isActive }

// CreatedBy returns the actor that created the benchmark set.
func (bs *BenchmarkSet) CreatedBy() string { return bs.createdBy }

// CreatedAt returns the creation timestamp.
func (bs *BenchmarkSet) CreatedAt() time.Time { return bs.createdAt }

// Tasks returns the ordered task references.
func (bs *BenchmarkSet) Tasks() []BenchmarkTask { return append([]BenchmarkTask(nil), bs.tasks...) }

// WithTasks returns a copy of the benchmark set with the given tasks.
func (bs *BenchmarkSet) WithTasks(tasks []BenchmarkTask) *BenchmarkSet {
	copy := *bs
	copy.tasks = append([]BenchmarkTask(nil), tasks...)
	return &copy
}

// SetActive marks the benchmark set as active.
func (bs *BenchmarkSet) SetActive() {
	bs.isActive = true
}

// SetInactive marks the benchmark set as inactive.
func (bs *BenchmarkSet) SetInactive() {
	bs.isActive = false
}
