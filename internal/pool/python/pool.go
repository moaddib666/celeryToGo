// Package python provides an implementation of the pool interface for Python processes.
package python

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"celeryToGo/internal/pool"
	"celeryToGo/internal/protocol"
)

// ProcessPool is an implementation of the Pool interface for Python processes.
type ProcessPool struct {
	// config contains configuration for the pool.
	config pool.PoolConfig

	// protocol is the protocol to use for communication with processes.
	protocol protocol.Protocol

	// processes is a map of process IDs to processes.
	processes map[string]pool.Process

	// processesMutex protects access to processes.
	processesMutex sync.RWMutex

	// idleProcesses is a list of idle process IDs.
	idleProcesses []string

	// idleProcessesMutex protects access to idleProcesses.
	idleProcessesMutex sync.RWMutex

	// stats contains statistics about the pool.
	stats pool.PoolStats

	// statsMutex protects access to stats.
	statsMutex sync.RWMutex

	// logger is the logger for the pool.
	logger *logrus.Entry

	// taskQueue is a channel for tasks to be executed.
	taskQueue chan taskRequest

	// shutdown is a channel for shutdown signals.
	shutdown chan struct{}

	// wg is a wait group for worker goroutines.
	wg sync.WaitGroup
}

// taskRequest represents a request to execute a task.
type taskRequest struct {
	// task is the task to execute.
	task protocol.TaskPayload

	// resultChan is the channel to send the result to.
	resultChan chan<- taskResult
}

// taskResult represents the result of a task execution.
type taskResult struct {
	// result is the result of the task execution.
	result protocol.ResultPayload

	// err is any error that occurred during task execution.
	err error
}

// NewProcessPool creates a new ProcessPool.
func NewProcessPool(config pool.PoolConfig, protocol protocol.Protocol) (*ProcessPool, error) {
	// Validate configuration
	if config.MinProcesses <= 0 {
		config.MinProcesses = 1
	}
	if config.MaxProcesses <= 0 {
		config.MaxProcesses = 10
	}
	if config.MaxProcesses < config.MinProcesses {
		config.MaxProcesses = config.MinProcesses
	}
	if config.MaxTasksPerProcess <= 0 {
		config.MaxTasksPerProcess = 100
	}
	if config.MaxIdleTime <= 0 {
		config.MaxIdleTime = 5 * time.Minute
	}
	if config.MaxProcessAge <= 0 {
		config.MaxProcessAge = 1 * time.Hour
	}
	if config.ProcessStartupTimeout <= 0 {
		config.ProcessStartupTimeout = 30 * time.Second
	}
	if config.ProcessShutdownTimeout <= 0 {
		config.ProcessShutdownTimeout = 30 * time.Second
	}
	if config.TaskExecutionTimeout <= 0 {
		config.TaskExecutionTimeout = 5 * time.Minute
	}

	// Create a logger for the pool
	logger := logrus.WithField("component", "python_process_pool")

	// Create the pool
	pool := &ProcessPool{
		config:        config,
		protocol:      protocol,
		processes:     make(map[string]pool.Process),
		idleProcesses: make([]string, 0),
		stats: pool.PoolStats{
			TotalProcesses:     0,
			IdleProcesses:      0,
			BusyProcesses:      0,
			StartingProcesses:  0,
			StoppingProcesses:  0,
			TotalTasksProcessed: 0,
			ProcessStats:       make(map[string]pool.ProcessStats),
		},
		logger:    logger,
		taskQueue: make(chan taskRequest),
		shutdown:  make(chan struct{}),
	}

	return pool, nil
}

// Start starts the pool.
func (p *ProcessPool) Start(ctx context.Context) error {
	p.logger.Info("Starting Python process pool")

	// Start worker goroutines
	for i := 0; i < p.config.MaxProcesses; i++ {
		p.wg.Add(1)
		go p.worker()
	}

	// Start the minimum number of processes
	for i := 0; i < p.config.MinProcesses; i++ {
		if err := p.createProcess(ctx); err != nil {
			p.logger.WithError(err).Error("Failed to create initial process")
			return fmt.Errorf("failed to create initial process: %w", err)
		}
	}

	// Start the process monitor
	p.wg.Add(1)
	go p.monitor()

	p.logger.Info("Python process pool started")
	return nil
}

// Stop stops the pool.
func (p *ProcessPool) Stop(ctx context.Context) error {
	p.logger.Info("Stopping Python process pool")

	// Signal all worker goroutines to stop
	close(p.shutdown)

	// Wait for all worker goroutines to stop with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		p.logger.Info("All worker goroutines stopped")
	case <-ctx.Done():
		p.logger.Warn("Timeout waiting for worker goroutines to stop")
	}

	// Stop all processes
	p.processesMutex.Lock()
	processes := make([]pool.Process, 0, len(p.processes))
	for _, process := range p.processes {
		processes = append(processes, process)
	}
	p.processesMutex.Unlock()

	for _, process := range processes {
		// Create a context with timeout for stopping the process
		stopCtx, cancel := context.WithTimeout(ctx, p.config.ProcessShutdownTimeout)
		defer cancel()

		// Stop the process
		if err := process.Stop(stopCtx, true); err != nil {
			p.logger.WithError(err).WithField("process_id", process.ID()).Warn("Failed to stop process gracefully")

			// Try to force stop the process
			if err := process.Stop(ctx, false); err != nil {
				p.logger.WithError(err).WithField("process_id", process.ID()).Error("Failed to force stop process")
			}
		}
	}

	p.logger.Info("Python process pool stopped")
	return nil
}

// ExecuteTask executes a task using a process from the pool.
func (p *ProcessPool) ExecuteTask(ctx context.Context, task protocol.TaskPayload) (protocol.ResultPayload, error) {
	p.logger.WithFields(logrus.Fields{
		"task_id":   task.TaskID,
		"task_name": task.TaskName,
	}).Debug("Executing task")

	// Create a channel for the result
	resultChan := make(chan taskResult, 1)

	// Send the task to the task queue
	select {
	case p.taskQueue <- taskRequest{task: task, resultChan: resultChan}:
		// Task sent to queue
	case <-ctx.Done():
		return protocol.ResultPayload{}, fmt.Errorf("context canceled while waiting to send task to queue: %w", ctx.Err())
	}

	// Wait for the result
	select {
	case result := <-resultChan:
		// Update stats
		p.statsMutex.Lock()
		p.stats.TotalTasksProcessed++
		p.statsMutex.Unlock()

		return result.result, result.err
	case <-ctx.Done():
		return protocol.ResultPayload{}, fmt.Errorf("context canceled while waiting for task result: %w", ctx.Err())
	}
}

// Stats returns statistics about the pool.
func (p *ProcessPool) Stats() pool.PoolStats {
	p.statsMutex.RLock()
	defer p.statsMutex.RUnlock()

	// Create a copy of the stats
	stats := pool.PoolStats{
		TotalProcesses:     p.stats.TotalProcesses,
		IdleProcesses:      p.stats.IdleProcesses,
		BusyProcesses:      p.stats.BusyProcesses,
		StartingProcesses:  p.stats.StartingProcesses,
		StoppingProcesses:  p.stats.StoppingProcesses,
		TotalTasksProcessed: p.stats.TotalTasksProcessed,
		ProcessStats:       make(map[string]pool.ProcessStats),
	}

	// Copy process stats
	for id, processStats := range p.stats.ProcessStats {
		stats.ProcessStats[id] = processStats
	}

	return stats
}

// worker is a goroutine that processes tasks from the task queue.
func (p *ProcessPool) worker() {
	defer p.wg.Done()

	for {
		select {
		case <-p.shutdown:
			return
		case req := <-p.taskQueue:
			// Get an idle process or create a new one
			process, err := p.getIdleProcess(context.Background())
			if err != nil {
				p.logger.WithError(err).Error("Failed to get idle process")
				req.resultChan <- taskResult{err: fmt.Errorf("failed to get idle process: %w", err)}
				continue
			}

			// Execute the task
			result, err := process.ExecuteTask(context.Background(), req.task)

			// Send the result
			req.resultChan <- taskResult{result: result, err: err}

			// Update process stats
			p.updateProcessStats(process.ID())

			// Check if the process should be recycled
			if p.shouldRecycleProcess(process.ID()) {
				p.recycleProcess(context.Background(), process.ID())
			} else {
				// Return the process to the idle pool
				p.returnProcessToIdlePool(process.ID())
			}
		}
	}
}

// monitor is a goroutine that monitors processes and recycles them as needed.
func (p *ProcessPool) monitor() {
	defer p.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-p.shutdown:
			return
		case <-ticker.C:
			p.checkAndRecycleProcesses()
		}
	}
}

// checkAndRecycleProcesses checks all processes and recycles them if needed.
func (p *ProcessPool) checkAndRecycleProcesses() {
	p.processesMutex.RLock()
	processes := make([]pool.Process, 0, len(p.processes))
	for _, process := range p.processes {
		processes = append(processes, process)
	}
	p.processesMutex.RUnlock()

	for _, process := range processes {
		if p.shouldRecycleProcess(process.ID()) {
			p.recycleProcess(context.Background(), process.ID())
		}
	}

	// Ensure we have the minimum number of processes
	p.ensureMinProcesses(context.Background())
}

// ensureMinProcesses ensures that we have at least the minimum number of processes.
func (p *ProcessPool) ensureMinProcesses(ctx context.Context) {
	p.processesMutex.RLock()
	numProcesses := len(p.processes)
	p.processesMutex.RUnlock()

	for i := numProcesses; i < p.config.MinProcesses; i++ {
		if err := p.createProcess(ctx); err != nil {
			p.logger.WithError(err).Error("Failed to create process to maintain minimum")
		}
	}
}

// createProcess creates a new Python process.
func (p *ProcessPool) createProcess(ctx context.Context) error {
	p.logger.Debug("Creating new Python process")

	// Check if we've reached the maximum number of processes
	p.processesMutex.RLock()
	if len(p.processes) >= p.config.MaxProcesses {
		p.processesMutex.RUnlock()
		return fmt.Errorf("maximum number of processes reached: %d", p.config.MaxProcesses)
	}
	p.processesMutex.RUnlock()

	// Update stats
	p.statsMutex.Lock()
	p.stats.StartingProcesses++
	p.statsMutex.Unlock()

	// Create a new process
	process, err := NewPythonProcess(PythonProcessConfig{
		PythonPath:       p.config.PythonPath,
		WorkerScriptPath: p.config.WorkerScriptPath,
		Env:              p.config.ProcessEnv,
		Protocol:         p.protocol,
		TaskTimeout:      p.config.TaskExecutionTimeout,
	})
	if err != nil {
		p.statsMutex.Lock()
		p.stats.StartingProcesses--
		p.statsMutex.Unlock()
		return fmt.Errorf("failed to create Python process: %w", err)
	}

	// Create a context with timeout for starting the process
	startCtx, cancel := context.WithTimeout(ctx, p.config.ProcessStartupTimeout)
	defer cancel()

	// Start the process
	if err := process.Start(startCtx); err != nil {
		p.statsMutex.Lock()
		p.stats.StartingProcesses--
		p.statsMutex.Unlock()
		return fmt.Errorf("failed to start Python process: %w", err)
	}

	// Add the process to the pool
	p.processesMutex.Lock()
	p.processes[process.ID()] = process
	p.processesMutex.Unlock()

	// Update stats
	p.statsMutex.Lock()
	p.stats.TotalProcesses++
	p.stats.IdleProcesses++
	p.stats.StartingProcesses--
	p.stats.ProcessStats[process.ID()] = process.Stats()
	p.statsMutex.Unlock()

	// Add the process to the idle pool
	p.idleProcessesMutex.Lock()
	p.idleProcesses = append(p.idleProcesses, process.ID())
	p.idleProcessesMutex.Unlock()

	p.logger.WithField("process_id", process.ID()).Info("Python process created and started")

	return nil
}

// getIdleProcess returns an idle process from the pool or creates a new one.
func (p *ProcessPool) getIdleProcess(ctx context.Context) (pool.Process, error) {
	// Try to get an idle process
	p.idleProcessesMutex.Lock()
	if len(p.idleProcesses) > 0 {
		// Get the first idle process
		processID := p.idleProcesses[0]
		p.idleProcesses = p.idleProcesses[1:]
		p.idleProcessesMutex.Unlock()

		// Get the process
		p.processesMutex.RLock()
		process, exists := p.processes[processID]
		p.processesMutex.RUnlock()

		if exists {
			// Update stats
			p.statsMutex.Lock()
			p.stats.IdleProcesses--
			p.stats.BusyProcesses++
			p.statsMutex.Unlock()

			p.logger.WithField("process_id", processID).Debug("Got idle process from pool")
			return process, nil
		}

		// Process doesn't exist, try again
		return p.getIdleProcess(ctx)
	}
	p.idleProcessesMutex.Unlock()

	// No idle processes, create a new one
	if err := p.createProcess(ctx); err != nil {
		return nil, fmt.Errorf("failed to create new process: %w", err)
	}

	// Try again
	return p.getIdleProcess(ctx)
}

// returnProcessToIdlePool returns a process to the idle pool.
func (p *ProcessPool) returnProcessToIdlePool(processID string) {
	p.logger.WithField("process_id", processID).Debug("Returning process to idle pool")

	// Add the process to the idle pool
	p.idleProcessesMutex.Lock()
	p.idleProcesses = append(p.idleProcesses, processID)
	p.idleProcessesMutex.Unlock()

	// Update stats
	p.statsMutex.Lock()
	p.stats.IdleProcesses++
	p.stats.BusyProcesses--
	p.statsMutex.Unlock()
}

// recycleProcess recycles a process.
func (p *ProcessPool) recycleProcess(ctx context.Context, processID string) {
	p.logger.WithField("process_id", processID).Info("Recycling process")

	// Get the process
	p.processesMutex.RLock()
	process, exists := p.processes[processID]
	p.processesMutex.RUnlock()

	if !exists {
		p.logger.WithField("process_id", processID).Warn("Process not found for recycling")
		return
	}

	// Remove the process from the idle pool
	p.idleProcessesMutex.Lock()
	for i, id := range p.idleProcesses {
		if id == processID {
			p.idleProcesses = append(p.idleProcesses[:i], p.idleProcesses[i+1:]...)
			break
		}
	}
	p.idleProcessesMutex.Unlock()

	// Update stats
	p.statsMutex.Lock()
	p.stats.StoppingProcesses++
	if process.Stats().State == pool.ProcessStateIdle {
		p.stats.IdleProcesses--
	} else if process.Stats().State == pool.ProcessStateBusy {
		p.stats.BusyProcesses--
	}
	p.statsMutex.Unlock()

	// Stop the process
	stopCtx, cancel := context.WithTimeout(ctx, p.config.ProcessShutdownTimeout)
	defer cancel()

	if err := process.Stop(stopCtx, true); err != nil {
		p.logger.WithError(err).WithField("process_id", processID).Warn("Failed to stop process gracefully")

		// Try to force stop the process
		if err := process.Stop(ctx, false); err != nil {
			p.logger.WithError(err).WithField("process_id", processID).Error("Failed to force stop process")
		}
	}

	// Remove the process from the pool
	p.processesMutex.Lock()
	delete(p.processes, processID)
	p.processesMutex.Unlock()

	// Update stats
	p.statsMutex.Lock()
	p.stats.TotalProcesses--
	p.stats.StoppingProcesses--
	delete(p.stats.ProcessStats, processID)
	p.statsMutex.Unlock()

	// Create a new process if needed
	p.ensureMinProcesses(ctx)
}

// shouldRecycleProcess checks if a process should be recycled.
func (p *ProcessPool) shouldRecycleProcess(processID string) bool {
	// Get the process stats
	p.statsMutex.RLock()
	stats, exists := p.stats.ProcessStats[processID]
	p.statsMutex.RUnlock()

	if !exists {
		return false
	}

	// Check if the process has processed too many tasks
	if p.config.MaxTasksPerProcess > 0 && stats.TasksProcessed >= p.config.MaxTasksPerProcess {
		p.logger.WithFields(logrus.Fields{
			"process_id":      processID,
			"tasks_processed": stats.TasksProcessed,
			"max_tasks":       p.config.MaxTasksPerProcess,
		}).Debug("Recycling process due to max tasks")
		return true
	}

	// Check if the process has been idle for too long
	if p.config.MaxIdleTime > 0 && stats.State == pool.ProcessStateIdle && stats.LastTaskTime.Add(p.config.MaxIdleTime).Before(time.Now()) {
		p.logger.WithFields(logrus.Fields{
			"process_id":    processID,
			"last_task_time": stats.LastTaskTime,
			"max_idle_time": p.config.MaxIdleTime,
		}).Debug("Recycling process due to idle timeout")
		return true
	}

	// Check if the process is too old
	if p.config.MaxProcessAge > 0 && stats.StartTime.Add(p.config.MaxProcessAge).Before(time.Now()) {
		p.logger.WithFields(logrus.Fields{
			"process_id":     processID,
			"start_time":     stats.StartTime,
			"max_process_age": p.config.MaxProcessAge,
		}).Debug("Recycling process due to age")
		return true
	}

	// Check if the process is in an error state
	if stats.State == pool.ProcessStateError {
		p.logger.WithField("process_id", processID).Debug("Recycling process due to error state")
		return true
	}

	return false
}

// updateProcessStats updates the stats for a process.
func (p *ProcessPool) updateProcessStats(processID string) {
	// Get the process
	p.processesMutex.RLock()
	process, exists := p.processes[processID]
	p.processesMutex.RUnlock()

	if !exists {
		return
	}

	// Update stats
	p.statsMutex.Lock()
	p.stats.ProcessStats[processID] = process.Stats()
	p.statsMutex.Unlock()
}
