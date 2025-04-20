// Package pool provides interfaces and implementations for process pools.
package pool

import (
	"context"
	"time"

	"celeryToGo/internal/protocol"
)

// ProcessState represents the state of a process.
type ProcessState string

const (
	// ProcessStateIdle means the process is running but not executing a task.
	ProcessStateIdle ProcessState = "idle"

	// ProcessStateBusy means the process is executing a task.
	ProcessStateBusy ProcessState = "busy"

	// ProcessStateStarting means the process is starting up.
	ProcessStateStarting ProcessState = "starting"

	// ProcessStateStopping means the process is shutting down.
	ProcessStateStopping ProcessState = "stopping"

	// ProcessStateStopped means the process has stopped.
	ProcessStateStopped ProcessState = "stopped"

	// ProcessStateError means the process has encountered an error.
	ProcessStateError ProcessState = "error"
)

// ProcessStats contains statistics about a process.
type ProcessStats struct {
	// PID is the process ID.
	PID int

	// State is the current state of the process.
	State ProcessState

	// StartTime is when the process was started.
	StartTime time.Time

	// TasksProcessed is the number of tasks processed by this process.
	TasksProcessed int64

	// LastTaskTime is when the last task was processed.
	LastTaskTime time.Time

	// CurrentTaskID is the ID of the task currently being processed (if any).
	CurrentTaskID string

	// CurrentTaskStartTime is when the current task started.
	CurrentTaskStartTime time.Time

	// MemoryUsage is the current memory usage of the process in bytes.
	MemoryUsage int64

	// CPUUsage is the current CPU usage of the process (0-100).
	CPUUsage float64
}

// Process represents a Python process that can execute tasks.
type Process interface {
	// Start starts the process.
	Start(ctx context.Context) error

	// Stop stops the process.
	Stop(ctx context.Context, graceful bool) error

	// ExecuteTask sends a task to the process for execution and returns the result.
	ExecuteTask(ctx context.Context, task protocol.TaskPayload) (protocol.ResultPayload, error)

	// Ping checks if the process is alive.
	Ping(ctx context.Context) error

	// Stats returns statistics about the process.
	Stats() ProcessStats

	// ID returns the unique identifier of the process.
	ID() string
}

// PoolConfig contains configuration for a process pool.
type PoolConfig struct {
	// MinProcesses is the minimum number of processes to keep in the pool.
	MinProcesses int

	// MaxProcesses is the maximum number of processes to allow in the pool.
	MaxProcesses int

	// MaxTasksPerProcess is the maximum number of tasks a process can execute before being recycled.
	MaxTasksPerProcess int64

	// MaxIdleTime is the maximum time a process can be idle before being recycled.
	MaxIdleTime time.Duration

	// MaxProcessAge is the maximum age of a process before being recycled.
	MaxProcessAge time.Duration

	// ProcessStartupTimeout is the maximum time to wait for a process to start.
	ProcessStartupTimeout time.Duration

	// ProcessShutdownTimeout is the maximum time to wait for a process to shut down gracefully.
	ProcessShutdownTimeout time.Duration

	// TaskExecutionTimeout is the default timeout for task execution.
	TaskExecutionTimeout time.Duration

	// ProcessEnv contains environment variables to pass to the processes.
	ProcessEnv []string

	// PythonPath is the path to the Python executable.
	PythonPath string

	// WorkerScriptPath is the path to the Python worker script.
	WorkerScriptPath string
}

// Pool defines the interface for process pools.
type Pool interface {
	// Start starts the pool.
	Start(ctx context.Context) error

	// Stop stops the pool.
	Stop(ctx context.Context) error

	// ExecuteTask executes a task using a process from the pool.
	ExecuteTask(ctx context.Context, task protocol.TaskPayload) (protocol.ResultPayload, error)

	// Stats returns statistics about the pool.
	Stats() PoolStats
}

// PoolStats contains statistics about a process pool.
type PoolStats struct {
	// TotalProcesses is the total number of processes in the pool.
	TotalProcesses int

	// IdleProcesses is the number of idle processes in the pool.
	IdleProcesses int

	// BusyProcesses is the number of busy processes in the pool.
	BusyProcesses int

	// StartingProcesses is the number of processes that are starting up.
	StartingProcesses int

	// StoppingProcesses is the number of processes that are shutting down.
	StoppingProcesses int

	// TotalTasksProcessed is the total number of tasks processed by the pool.
	TotalTasksProcessed int64

	// ProcessStats contains statistics for each process in the pool.
	ProcessStats map[string]ProcessStats
}