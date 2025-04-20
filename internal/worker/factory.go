// Package worker provides interfaces and implementations for task workers.
package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"celeryToGo/internal/broker"
	"celeryToGo/internal/observability/logging"
	"celeryToGo/internal/pool"
	"celeryToGo/internal/protocol"
)

// DefaultWorker is the default implementation of the Worker interface.
type DefaultWorker struct {
	// broker is the message broker.
	broker broker.Broker

	// pool is the process pool.
	pool pool.Pool

	// protocol is the protocol for communication with processes.
	protocol protocol.Protocol

	// config is the configuration for the worker.
	config WorkerConfig

	// stats contains statistics about the worker.
	stats WorkerStats

	// taskInfos is a map of task IDs to task information.
	taskInfos map[string]TaskInfo

	// taskInfosMutex is a mutex to protect access to the taskInfos map.
	taskInfosMutex sync.Mutex

	// logger is the logger for the worker.
	logger logging.Logger

	// shutdown is a channel for shutdown signals.
	shutdown chan struct{}

	// done is a channel that is closed when the worker is stopped.
	done chan struct{}
}

// NewDefaultWorker creates a new DefaultWorker.
func NewDefaultWorker(
	broker broker.Broker,
	pool pool.Pool,
	protocol protocol.Protocol,
	config WorkerConfig,
) *DefaultWorker {
	return &DefaultWorker{
		broker:    broker,
		pool:      pool,
		protocol:  protocol,
		config:    config,
		taskInfos: make(map[string]TaskInfo),
		logger:    logging.WithField("component", "worker"),
		shutdown:  make(chan struct{}),
		done:      make(chan struct{}),
	}
}

// Start starts the worker.
func (w *DefaultWorker) Start(ctx context.Context) error {
	w.logger.Info("Starting worker")

	// Start a goroutine to process tasks
	go w.processTasksLoop(ctx)

	w.logger.Info("Worker started")
	return nil
}

// Stop stops the worker.
func (w *DefaultWorker) Stop(ctx context.Context) error {
	w.logger.Info("Stopping worker")

	// Signal the worker to stop
	close(w.shutdown)

	// Wait for the worker to stop or timeout
	select {
	case <-w.done:
		w.logger.Info("Worker stopped")
		return nil
	case <-ctx.Done():
		w.logger.Warn("Timeout waiting for worker to stop")
		return fmt.Errorf("timeout waiting for worker to stop")
	}
}

// Stats returns statistics about the worker.
func (w *DefaultWorker) Stats() WorkerStats {
	// Get pool stats
	poolStats := w.pool.Stats()

	// Update worker stats
	w.stats.ActiveTasks = poolStats.BusyProcesses
	w.stats.PoolStats = poolStats

	return w.stats
}

// GetTaskInfo returns information about a task.
func (w *DefaultWorker) GetTaskInfo(taskID string) (TaskInfo, error) {
	w.taskInfosMutex.Lock()
	defer w.taskInfosMutex.Unlock()

	taskInfo, ok := w.taskInfos[taskID]
	if !ok {
		return TaskInfo{}, fmt.Errorf("task not found: %s", taskID)
	}
	return taskInfo, nil
}

// processTasksLoop processes tasks from the broker.
func (w *DefaultWorker) processTasksLoop(ctx context.Context) {
	defer close(w.done)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Context canceled, stopping worker")
			return
		case <-w.shutdown:
			w.logger.Info("Shutdown signal received, stopping worker")
			return
		default:
			// Process tasks
			if err := w.processTasks(ctx); err != nil {
				w.logger.WithError(err).Error("Failed to process tasks")
			}
		}
	}
}

// processTasks processes tasks from the broker.
func (w *DefaultWorker) processTasks(ctx context.Context) error {
	// Receive messages from the broker
	messages, err := w.broker.ReceiveMessages(ctx, w.config.QueuePrefetch)
	if err != nil {
		return fmt.Errorf("failed to receive messages: %w", err)
	}

	// Process each message
	for _, message := range messages {
		// Create a task payload from the message
		taskPayload, err := w.protocol.DecodeTask(ctx, message.Body)
		if err != nil {
			w.logger.WithError(err).Error("Failed to decode task", message.ID, string(message.Body))
			continue
		}

		// Create a task info
		taskInfo := TaskInfo{
			ID:             taskPayload.TaskID,
			Name:           taskPayload.TaskName,
			State:          TaskStatePending,
			Args:           taskPayload.Args,
			Kwargs:         taskPayload.Kwargs,
			Queue:          taskPayload.Queue,
			MessageGroupID: message.MessageGroupID,
			Retries:        taskPayload.Retries,
			ReceivedAt:     time.Now(),
		}

		// Store the task info
		w.taskInfosMutex.Lock()
		w.taskInfos[taskInfo.ID] = taskInfo
		w.taskInfosMutex.Unlock()

		// Update stats
		w.stats.TasksReceived++

		// Execute the task
		go w.executeTask(ctx, taskPayload, message)
	}

	return nil
}

// executeTask executes a task.
func (w *DefaultWorker) executeTask(ctx context.Context, taskPayload protocol.TaskPayload, message broker.Message) {
	// Update task info
	w.taskInfosMutex.Lock()
	taskInfo := w.taskInfos[taskPayload.TaskID]
	taskInfo.State = TaskStateRunning
	taskInfo.StartedAt = time.Now()
	w.taskInfos[taskPayload.TaskID] = taskInfo
	w.taskInfosMutex.Unlock()

	// Execute the task
	result, err := w.pool.ExecuteTask(ctx, taskPayload)
	if err != nil {
		w.logger.WithError(err).WithField("task_id", taskPayload.TaskID).Error("Failed to execute task")

		// Update task info
		w.taskInfosMutex.Lock()
		taskInfo := w.taskInfos[taskPayload.TaskID]
		taskInfo.State = TaskStateError
		taskInfo.Error = err.Error()
		taskInfo.CompletedAt = time.Now()
		w.taskInfos[taskPayload.TaskID] = taskInfo
		w.taskInfosMutex.Unlock()

		// Update stats
		w.stats.TasksFailed++

		// Acknowledge the message
		if err := w.broker.AcknowledgeMessage(ctx, message); err != nil {
			w.logger.WithError(err).WithField("task_id", taskPayload.TaskID).Error("Failed to acknowledge message")
		}

		return
	}

	// Update task info
	w.taskInfosMutex.Lock()
	taskInfo = w.taskInfos[taskPayload.TaskID]
	taskInfo.CompletedAt = time.Now()
	taskInfo.Result = result.Result

	if result.Status == "success" {
		taskInfo.State = TaskStateSuccess
		w.stats.TasksSucceeded++
	} else {
		taskInfo.State = TaskStateError
		taskInfo.Error = result.Error
		taskInfo.Traceback = result.Traceback
		w.stats.TasksFailed++
	}

	w.taskInfos[taskPayload.TaskID] = taskInfo
	w.taskInfosMutex.Unlock()

	// Acknowledge the message
	if err := w.broker.AcknowledgeMessage(ctx, message); err != nil {
		w.logger.WithError(err).WithField("task_id", taskPayload.TaskID).Error("Failed to acknowledge message")
	}
}

// DefaultWorkerFactory is the default implementation of the WorkerFactory interface.
type DefaultWorkerFactory struct{}

// NewDefaultWorkerFactory creates a new DefaultWorkerFactory.
func NewDefaultWorkerFactory() *DefaultWorkerFactory {
	return &DefaultWorkerFactory{}
}

// CreateWorker creates a new worker with the given configuration.
func (f *DefaultWorkerFactory) CreateWorker(
	ctx context.Context,
	broker broker.Broker,
	pool pool.Pool,
	protocol protocol.Protocol,
	config WorkerConfig,
) (Worker, error) {
	return NewDefaultWorker(broker, pool, protocol, config), nil
}
