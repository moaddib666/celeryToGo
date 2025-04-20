// Package worker provides interfaces and implementations for task workers.
package worker

import (
	"context"
	"time"

	"celeryToGo/internal/broker"
	"celeryToGo/internal/pool"
	"celeryToGo/internal/protocol"
)

// TaskState represents the state of a task.
type TaskState string

const (
	// TaskStatePending means the task is waiting to be executed.
	TaskStatePending TaskState = "pending"

	// TaskStateRunning means the task is currently being executed.
	TaskStateRunning TaskState = "running"

	// TaskStateSuccess means the task was executed successfully.
	TaskStateSuccess TaskState = "success"

	// TaskStateError means the task execution failed with an error.
	TaskStateError TaskState = "error"

	// TaskStateRetry means the task will be retried.
	TaskStateRetry TaskState = "retry"
)

// TaskInfo contains information about a task.
type TaskInfo struct {
	// ID is the unique identifier of the task.
	ID string

	// Name is the name of the task.
	Name string

	// State is the current state of the task.
	State TaskState

	// Args are the positional arguments for the task.
	Args []interface{}

	// Kwargs are the keyword arguments for the task.
	Kwargs map[string]interface{}

	// Queue is the name of the queue this task was received from.
	Queue string

	// MessageGroupID is the message group ID for FIFO queues.
	MessageGroupID string

	// Retries is the number of times this task has been retried.
	Retries int

	// MaxRetries is the maximum number of retries for this task.
	MaxRetries int

	// ReceivedAt is when the task was received.
	ReceivedAt time.Time

	// StartedAt is when the task execution started.
	StartedAt time.Time

	// CompletedAt is when the task execution completed.
	CompletedAt time.Time

	// Result is the result of the task execution (if successful).
	Result interface{}

	// Error is the error message (if the task execution failed).
	Error string

	// Traceback is the traceback of the error (if the task execution failed).
	Traceback string

	// ProcessID is the ID of the process that executed the task.
	ProcessID string
}

// WorkerConfig contains configuration for a worker.
type WorkerConfig struct {
	// Concurrency is the maximum number of tasks to process concurrently.
	Concurrency int

	// QueuePrefetch is the number of messages to prefetch from the queue.
	QueuePrefetch int

	// TaskTimeout is the default timeout for task execution.
	TaskTimeout time.Duration

	// ShutdownTimeout is the maximum time to wait for tasks to complete during shutdown.
	ShutdownTimeout time.Duration

	// RetryBackoff is the backoff strategy for retrying tasks.
	RetryBackoff RetryBackoff

	// HeartbeatInterval is the interval at which to send heartbeats.
	HeartbeatInterval time.Duration
}

// RetryBackoff defines the backoff strategy for retrying tasks.
type RetryBackoff struct {
	// InitialInterval is the initial interval between retries.
	InitialInterval time.Duration

	// MaxInterval is the maximum interval between retries.
	MaxInterval time.Duration

	// Multiplier is the factor by which the interval increases.
	Multiplier float64

	// RandomizationFactor is the randomization factor to apply to the interval.
	RandomizationFactor float64
}

// WorkerStats contains statistics about a worker.
type WorkerStats struct {
	// StartTime is when the worker was started.
	StartTime time.Time

	// TasksReceived is the number of tasks received.
	TasksReceived int64

	// TasksSucceeded is the number of tasks that succeeded.
	TasksSucceeded int64

	// TasksFailed is the number of tasks that failed.
	TasksFailed int64

	// TasksRetried is the number of tasks that were retried.
	TasksRetried int64

	// ActiveTasks is the number of tasks currently being processed.
	ActiveTasks int

	// PoolStats contains statistics about the process pool.
	PoolStats pool.PoolStats
}

// Worker defines the interface for task workers.
type Worker interface {
	// Start starts the worker.
	Start(ctx context.Context) error

	// Stop stops the worker.
	Stop(ctx context.Context) error

	// Stats returns statistics about the worker.
	Stats() WorkerStats

	// GetTaskInfo returns information about a task.
	GetTaskInfo(taskID string) (TaskInfo, error)
}

// WorkerFactory creates a new worker.
type WorkerFactory interface {
	// CreateWorker creates a new worker with the given configuration.
	CreateWorker(
		ctx context.Context,
		broker broker.Broker,
		pool pool.Pool,
		protocol protocol.Protocol,
		config WorkerConfig,
	) (Worker, error)
}
