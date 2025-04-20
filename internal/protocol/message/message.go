// Package message provides message definitions for the IPC protocol.
package message

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"celeryToGo/internal/protocol"
)

// BaseMessage is the base struct for all messages.
type BaseMessage struct {
	// MessageType is the type of the message.
	MessageType string `json:"type"`

	// MessageID is the unique identifier of the message.
	MessageID string `json:"id"`

	// MessageTimestamp is the time when the message was created.
	MessageTimestamp time.Time `json:"timestamp"`
}

// NewBaseMessage creates a new BaseMessage with the given type.
func NewBaseMessage(msgType string) BaseMessage {
	return BaseMessage{
		MessageType:      msgType,
		MessageID:        uuid.New().String(),
		MessageTimestamp: time.Now(),
	}
}

// Type returns the type of the message.
func (m BaseMessage) Type() string {
	return m.MessageType
}

// ID returns the unique identifier of the message.
func (m BaseMessage) ID() string {
	return m.MessageID
}

// Timestamp returns the time when the message was created.
func (m BaseMessage) Timestamp() time.Time {
	return m.MessageTimestamp
}

// TaskMessage is a message containing a task to be executed.
type TaskMessage struct {
	BaseMessage
	// TaskName is the name of the task to execute.
	TaskName string `json:"task_name"`

	// TaskID is the unique identifier for this task execution.
	TaskID string `json:"task_id"`

	// Args are the positional arguments for the task.
	Args []interface{} `json:"args"`

	// Kwargs are the keyword arguments for the task.
	Kwargs map[string]interface{} `json:"kwargs"`

	// Retries is the number of times this task has been retried.
	Retries int `json:"retries"`

	// Queue is the name of the queue this task was received from.
	Queue string `json:"queue"`
}

// NewTaskMessage creates a new TaskMessage with the given task payload.
func NewTaskMessage(payload protocol.TaskPayload) TaskMessage {
	return TaskMessage{
		BaseMessage: NewBaseMessage(protocol.InternalTaskMessage),
		TaskName:    payload.TaskName,
		TaskID:      payload.TaskID,
		Args:        payload.Args,
		Kwargs:      payload.Kwargs,
		Retries:     payload.Retries,
		Queue:       payload.Queue,
	}
}

// ToPayload converts the TaskMessage to a TaskPayload.
func (m TaskMessage) ToPayload() protocol.TaskPayload {
	return protocol.TaskPayload{
		TaskName: m.TaskName,
		TaskID:   m.TaskID,
		Args:     m.Args,
		Kwargs:   m.Kwargs,
		Retries:  m.Retries,
		Queue:    m.Queue,
	}
}

// ResultMessage is a message containing the result of a task execution.
type ResultMessage struct {
	BaseMessage
	// TaskID is the unique identifier of the task that was executed.
	TaskID string `json:"task_id"`

	// Status is the status of the task execution (success, error, etc.).
	Status string `json:"status"`

	// Result is the result of the task execution (if successful).
	Result interface{} `json:"result,omitempty"`

	// Error is the error message (if the task execution failed).
	Error string `json:"error,omitempty"`

	// Traceback is the traceback of the error (if the task execution failed).
	Traceback string `json:"traceback,omitempty"`

	// Runtime is the duration of the task execution in seconds.
	Runtime float64 `json:"runtime_seconds"`

	// MemoryUsed is the amount of memory used by the task execution in bytes.
	MemoryUsed int64 `json:"memory_used,omitempty"`
}

// NewResultMessage creates a new ResultMessage with the given result payload.
func NewResultMessage(payload protocol.ResultPayload) ResultMessage {
	return ResultMessage{
		BaseMessage: NewBaseMessage(protocol.InternalResultMessage),
		TaskID:      payload.TaskID,
		Status:      payload.Status,
		Result:      payload.Result,
		Error:       payload.Error,
		Traceback:   payload.Traceback,
		Runtime:     payload.Runtime,
		MemoryUsed:  payload.MemoryUsed,
	}
}

// ToPayload converts the ResultMessage to a ResultPayload.
func (m ResultMessage) ToPayload() protocol.ResultPayload {
	return protocol.ResultPayload{
		TaskID:     m.TaskID,
		Status:     m.Status,
		Result:     m.Result,
		Error:      m.Error,
		Traceback:  m.Traceback,
		Runtime:    m.Runtime,
		MemoryUsed: m.MemoryUsed,
	}
}

// PingMessage is a message used to check if a process is alive.
type PingMessage struct {
	BaseMessage
}

// NewPingMessage creates a new PingMessage.
func NewPingMessage() PingMessage {
	return PingMessage{
		BaseMessage: NewBaseMessage(protocol.InternalPingMessage),
	}
}

// PongMessage is a response to a ping message.
type PongMessage struct {
	BaseMessage
	// PingID is the ID of the ping message this is responding to.
	PingID string `json:"ping_id"`
}

// NewPongMessage creates a new PongMessage in response to a ping message.
func NewPongMessage(pingID string) PongMessage {
	return PongMessage{
		BaseMessage: NewBaseMessage(protocol.InternalPongMessage),
		PingID:      pingID,
	}
}

// StopMessage is a message requesting a process to stop.
type StopMessage struct {
	BaseMessage
	// Graceful indicates whether the process should stop gracefully.
	Graceful bool `json:"graceful"`
}

// NewStopMessage creates a new StopMessage.
func NewStopMessage(graceful bool) StopMessage {
	return StopMessage{
		BaseMessage: NewBaseMessage(protocol.InternalStopMessage),
		Graceful:    graceful,
	}
}

// ParseMessage parses a JSON message and returns the appropriate message type.
func ParseMessage(data []byte) (interface{}, error) {
	// Parse the base message to determine the type
	var base BaseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	// Parse the full message based on the type
	switch base.MessageType {
	case protocol.InternalTaskMessage:
		var msg TaskMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("failed to parse task message: %w", err)
		}
		return msg, nil
	case protocol.InternalResultMessage:
		var msg ResultMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("failed to parse result message: %w", err)
		}
		return msg, nil
	case protocol.InternalPingMessage:
		var msg PingMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("failed to parse ping message: %w", err)
		}
		return msg, nil
	case protocol.InternalPongMessage:
		var msg PongMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("failed to parse pong message: %w", err)
		}
		return msg, nil
	case protocol.InternalStopMessage:
		var msg StopMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("failed to parse stop message: %w", err)
		}
		return msg, nil
	default:
		return nil, fmt.Errorf("unknown message type: %s", base.Type())
	}
}
