// Package python provides an implementation of the pool interface for Python processes.
package python

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"celeryToGo/internal/pool"
	"celeryToGo/internal/protocol"
)

// PythonProcess is an implementation of the Process interface for Python processes.
type PythonProcess struct {
	// id is the unique identifier of the process.
	id string

	// cmd is the command to run the Python process.
	cmd *exec.Cmd

	// stdin is the stdin pipe for the process.
	stdin io.WriteCloser

	// stdout is the stdout pipe for the process.
	stdout io.ReadCloser

	// stderr is the stderr pipe for the process.
	stderr io.ReadCloser

	// protocol is the protocol to use for communication with the process.
	protocol protocol.Protocol

	// state is the current state of the process.
	state pool.ProcessState

	// stateMutex protects access to state.
	stateMutex sync.RWMutex

	// stats contains statistics about the process.
	stats pool.ProcessStats

	// statsMutex protects access to stats.
	statsMutex sync.RWMutex

	// logger is the logger for the process.
	logger *logrus.Entry

	// stderrLogger is used to log stderr output from the process.
	stderrLogger *logrus.Entry

	// taskTimeout is the default timeout for task execution.
	taskTimeout time.Duration
}

// PythonProcessConfig contains configuration for a Python process.
type PythonProcessConfig struct {
	// PythonPath is the path to the Python executable.
	PythonPath string

	// WorkerScriptPath is the path to the Python worker script.
	WorkerScriptPath string

	// Env contains environment variables to pass to the process.
	Env []string

	// Protocol is the protocol to use for communication with the process.
	Protocol protocol.Protocol

	// TaskTimeout is the default timeout for task execution.
	TaskTimeout time.Duration
}

// NewPythonProcess creates a new PythonProcess.
func NewPythonProcess(config PythonProcessConfig) (*PythonProcess, error) {
	// Generate a unique ID for the process
	id := uuid.New().String()

	// Create a logger for the process
	logger := logrus.WithFields(logrus.Fields{
		"component":  "python_process",
		"process_id": id,
	})

	// Create a stderr logger for the process
	stderrLogger := logger.WithField("source", "stderr")

	// Create the process
	process := &PythonProcess{
		id:           id,
		protocol:     config.Protocol,
		state:        pool.ProcessStateStarting,
		logger:       logger,
		stderrLogger: stderrLogger,
		taskTimeout:  config.TaskTimeout,
		stats: pool.ProcessStats{
			PID:            0,
			State:          pool.ProcessStateStarting,
			StartTime:      time.Now(),
			TasksProcessed: 0,
		},
	}

	// Set up the command
	pythonPath := config.PythonPath
	if pythonPath == "" {
		pythonPath = "python"
	}

	// Create the command
	process.cmd = exec.Command(pythonPath, config.WorkerScriptPath)

	// Set up environment variables
	if len(config.Env) > 0 {
		process.cmd.Env = append(os.Environ(), config.Env...)
	} else {
		process.cmd.Env = os.Environ()
	}

	// Add process ID to environment
	process.cmd.Env = append(process.cmd.Env, fmt.Sprintf("CELERY_GO_PROCESS_ID=%s", id))

	// Extend the PYTHONPATH to include the directory of current executable running directory using os.Executable()
	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}
	executableDir := filepath.Dir(executablePath)
	process.cmd.Env = append(process.cmd.Env, fmt.Sprintf("PYTHONPATH=%s:%s", executableDir, os.Getenv("PYTHONPATH")))

	// Setup the working directory
	process.cmd.Dir = executableDir

	// Set up process group for signal handling
	process.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	logger.WithFields(logrus.Fields{
		"python_path":        pythonPath,
		"worker_script_path": config.WorkerScriptPath,
	}).Debug("Created Python process")

	return process, nil
}

// Start starts the process.
func (p *PythonProcess) Start(ctx context.Context) error {
	p.logger.Info("Starting Python process")

	// Set up pipes for stdin, stdout, and stderr
	var err error
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		p.logger.WithError(err).Error("Failed to create stdin pipe")
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	p.stdout, err = p.cmd.StdoutPipe()
	if err != nil {
		p.logger.WithError(err).Error("Failed to create stdout pipe")
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	p.stderr, err = p.cmd.StderrPipe()
	if err != nil {
		p.logger.WithError(err).Error("Failed to create stderr pipe")
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := p.cmd.Start(); err != nil {
		p.logger.WithError(err).Error("Failed to start Python process")
		return fmt.Errorf("failed to start Python process: %w", err)
	}

	// Update state and stats
	p.setState(pool.ProcessStateIdle)
	p.statsMutex.Lock()
	p.stats.PID = p.cmd.Process.Pid
	p.stats.State = pool.ProcessStateIdle
	p.statsMutex.Unlock()

	p.logger.WithField("pid", p.cmd.Process.Pid).Info("Python process started")

	// Start goroutines to handle stdout and stderr
	go p.handleStdout()
	go p.handleStderr()

	// FIXME: As python runtime may take time to start, we can wait for a bit also when task is executing the channel is blocked
	// Send a ping to check if the process is ready
	//if err := p.Ping(ctx); err != nil {
	//	p.logger.WithError(err).Error("Failed to ping Python process")
	//	return fmt.Errorf("failed to ping Python process: %w", err)
	//}

	return nil
}

// Stop stops the process.
func (p *PythonProcess) Stop(ctx context.Context, graceful bool) error {
	p.logger.WithField("graceful", graceful).Info("Stopping Python process")

	// Update state
	p.setState(pool.ProcessStateStopping)

	if graceful {
		// Try to send a stop message
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		stopData, err := p.protocol.EncodeStop(stopCtx)
		if err != nil {
			p.logger.WithError(err).Warn("Failed to encode stop message")
		} else {
			// Write the stop message to stdin
			if _, err := p.stdin.Write(append(stopData, '\n')); err != nil {
				p.logger.WithError(err).Warn("Failed to send stop message")
			}
		}

		// Wait for the process to exit
		done := make(chan error, 1)
		go func() {
			done <- p.cmd.Wait()
		}()

		// Wait for the process to exit or timeout
		select {
		case err := <-done:
			if err != nil {
				p.logger.WithError(err).Warn("Python process exited with error")
			} else {
				p.logger.Info("Python process exited gracefully")
			}
		case <-ctx.Done():
			p.logger.Warn("Timeout waiting for Python process to exit gracefully, killing")
			graceful = false
		}
	}

	if !graceful {
		// Kill the process
		if p.cmd.Process != nil {
			// Kill the process group
			pgid, err := syscall.Getpgid(p.cmd.Process.Pid)
			if err == nil {
				if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
					p.logger.WithError(err).Warn("Failed to kill process group")
				}
			}

			// Kill the process
			if err := p.cmd.Process.Kill(); err != nil {
				p.logger.WithError(err).Warn("Failed to kill Python process")
			}
		}
	}

	// Close pipes
	if p.stdin != nil {
		p.stdin.Close()
	}
	if p.stdout != nil {
		p.stdout.Close()
	}
	if p.stderr != nil {
		p.stderr.Close()
	}

	// Update state and stats
	p.setState(pool.ProcessStateStopped)
	p.statsMutex.Lock()
	p.stats.State = pool.ProcessStateStopped
	p.statsMutex.Unlock()

	p.logger.Info("Python process stopped")

	return nil
}

// ExecuteTask sends a task to the process for execution and returns the result.
func (p *PythonProcess) ExecuteTask(ctx context.Context, task protocol.TaskPayload) (protocol.ResultPayload, error) {
	p.logger.WithFields(logrus.Fields{
		"task_id":   task.TaskID,
		"task_name": task.TaskName,
	}).Debug("Executing task")

	// Update state and stats
	p.setState(pool.ProcessStateBusy)
	p.statsMutex.Lock()
	p.stats.State = pool.ProcessStateBusy
	p.stats.CurrentTaskID = task.TaskID
	p.stats.CurrentTaskStartTime = time.Now()
	p.statsMutex.Unlock()

	// Create a context with timeout
	taskCtx, cancel := context.WithTimeout(ctx, p.taskTimeout)
	defer cancel()

	// Encode the task
	taskData, err := p.protocol.EncodeTask(taskCtx, task)
	if err != nil {
		p.logger.WithError(err).Error("Failed to encode task")
		return protocol.ResultPayload{}, fmt.Errorf("failed to encode task: %w", err)
	}

	// Write the task to stdin
	if _, err := p.stdin.Write(append(taskData, '\n')); err != nil {
		p.logger.WithError(err).Error("Failed to send task to Python process")
		return protocol.ResultPayload{}, fmt.Errorf("failed to send task to Python process: %w", err)
	}

	// Read the result from stdout
	resultChan := make(chan protocol.ResultPayload, 1)
	errChan := make(chan error, 1)

	go func() {
		// Read from stdout until we get a result
		scanner := bufio.NewScanner(p.stdout)
		for scanner.Scan() {
			line := scanner.Bytes()

			// Try to decode the result
			result, err := p.protocol.DecodeResult(taskCtx, line)
			if err != nil {
				// Not a result message, continue
				continue
			}

			// Check if this is the result for our task
			if result.TaskID == task.TaskID {
				resultChan <- result
				return
			}
		}

		if err := scanner.Err(); err != nil {
			errChan <- fmt.Errorf("error reading from stdout: %w", err)
		}
	}()

	// Wait for the result or timeout
	select {
	case result := <-resultChan:
		// Update state and stats
		p.setState(pool.ProcessStateIdle)
		p.statsMutex.Lock()
		p.stats.State = pool.ProcessStateIdle
		p.stats.TasksProcessed++
		p.stats.LastTaskTime = time.Now()
		p.stats.CurrentTaskID = ""
		p.stats.CurrentTaskStartTime = time.Time{}
		p.statsMutex.Unlock()

		p.logger.WithFields(logrus.Fields{
			"task_id":   task.TaskID,
			"task_name": task.TaskName,
			"status":    result.Status,
			"runtime":   result.Runtime,
		}).Debug("Task executed")

		return result, nil
	case err := <-errChan:
		// Update state and stats
		p.setState(pool.ProcessStateError)
		p.statsMutex.Lock()
		p.stats.State = pool.ProcessStateError
		p.stats.CurrentTaskID = ""
		p.stats.CurrentTaskStartTime = time.Time{}
		p.statsMutex.Unlock()

		p.logger.WithError(err).Error("Error executing task")
		return protocol.ResultPayload{}, err
	case <-taskCtx.Done():
		// Update state and stats
		p.setState(pool.ProcessStateError)
		p.statsMutex.Lock()
		p.stats.State = pool.ProcessStateError
		p.stats.CurrentTaskID = ""
		p.stats.CurrentTaskStartTime = time.Time{}
		p.statsMutex.Unlock()

		p.logger.WithField("task_id", task.TaskID).Error("Task execution timed out")
		return protocol.ResultPayload{}, fmt.Errorf("task execution timed out: %w", taskCtx.Err())
	}
}

// Ping checks if the process is alive.
func (p *PythonProcess) Ping(ctx context.Context) error {
	p.logger.Debug("Pinging Python process")

	// Create a context with timeout
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Encode the ping
	pingData, err := p.protocol.EncodePing(pingCtx)
	if err != nil {
		p.logger.WithError(err).Error("Failed to encode ping")
		return fmt.Errorf("failed to encode ping: %w", err)
	}

	// Write the ping to stdin
	if _, err := p.stdin.Write(append(pingData, '\n')); err != nil {
		p.logger.WithError(err).Error("Failed to send ping to Python process")
		return fmt.Errorf("failed to send ping to Python process: %w", err)
	}

	// Read the pong from stdout
	pongChan := make(chan struct{}, 1)
	errChan := make(chan error, 1)

	go func() {
		// Read from stdout until we get a pong
		scanner := bufio.NewScanner(p.stdout)
		for scanner.Scan() {
			line := scanner.Bytes()

			// Try to decode the pong
			if err := p.protocol.DecodePong(pingCtx, line); err == nil {
				pongChan <- struct{}{}
				return
			}
		}

		if err := scanner.Err(); err != nil {
			errChan <- fmt.Errorf("error reading from stdout: %w", err)
		}
	}()

	// Wait for the pong or timeout
	select {
	case <-pongChan:
		p.logger.Debug("Received pong from Python process")
		return nil
	case err := <-errChan:
		p.logger.WithError(err).Error("Error pinging Python process")
		return err
	case <-pingCtx.Done():
		p.logger.Error("Ping timed out")
		return fmt.Errorf("ping timed out: %w", pingCtx.Err())
	}
}

// Stats returns statistics about the process.
func (p *PythonProcess) Stats() pool.ProcessStats {
	p.statsMutex.RLock()
	defer p.statsMutex.RUnlock()
	return p.stats
}

// ID returns the unique identifier of the process.
func (p *PythonProcess) ID() string {
	return p.id
}

// setState updates the state of the process.
func (p *PythonProcess) setState(state pool.ProcessState) {
	p.stateMutex.Lock()
	defer p.stateMutex.Unlock()
	p.state = state
}

// getState returns the current state of the process.
func (p *PythonProcess) getState() pool.ProcessState {
	p.stateMutex.RLock()
	defer p.stateMutex.RUnlock()
	return p.state
}

// handleStdout reads from stdout and logs any output that is not a protocol message.
func (p *PythonProcess) handleStdout() {
	scanner := bufio.NewScanner(p.stdout)
	for scanner.Scan() {
		line := scanner.Text()

		// Try to parse the line as JSON
		var message map[string]interface{}
		if err := json.Unmarshal([]byte(line), &message); err == nil {
			// This is a JSON message, likely a protocol message
			// We don't need to log it
			continue
		}

		// This is not a JSON message, log it
		p.logger.WithField("stdout", line).Debug("Python process stdout")
	}

	if err := scanner.Err(); err != nil {
		p.logger.WithError(err).Warn("Error reading from stdout")
	}
}

// handleStderr reads from stderr and logs any output.
func (p *PythonProcess) handleStderr() {
	scanner := bufio.NewScanner(p.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		p.stderrLogger.WithField("stderr", line).Warn("Python process stderr")
	}

	if err := scanner.Err(); err != nil {
		p.logger.WithError(err).Warn("Error reading from stderr")
	}
}
