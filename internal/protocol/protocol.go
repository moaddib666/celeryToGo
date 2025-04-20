// Package protocol provides interfaces and implementations for IPC protocols.
package protocol

import (
	"context"
)

// Constants for internal message types (not part of Celery v2 protocol)
const (
	// InternalTaskMessage is a message containing a task to be executed.
	InternalTaskMessage = "task"

	// InternalResultMessage is a message containing the result of a task execution.
	InternalResultMessage = "result"

	// InternalPingMessage is a message used to check if a process is alive.
	InternalPingMessage = "ping"

	// InternalPongMessage is a response to a ping message.
	InternalPongMessage = "pong"

	// InternalStopMessage is a message requesting a process to stop.
	InternalStopMessage = "stop"
)

// CeleryMessage represents a Celery v2 protocol message.
type CeleryMessage struct {
	// Body contains the base64-encoded task data.
	Body string `json:"body"`

	// ContentEncoding is the encoding of the body (usually utf-8).
	ContentEncoding string `json:"content-encoding"`

	// ContentType is the content type of the body (usually application/json).
	ContentType string `json:"content-type"`

	// Headers contains task-related information.
	Headers map[string]interface{} `json:"headers"`

	// Properties contains delivery-related information.
	Properties map[string]interface{} `json:"properties"`
}

// TaskPayload contains the data needed to execute a task.
type TaskPayload struct {
	// TaskName is the name of the task to execute.
	TaskName string

	// TaskID is the unique identifier for this task execution.
	TaskID string

	// Args are the positional arguments for the task.
	Args []interface{}

	// Kwargs are the keyword arguments for the task.
	Kwargs map[string]interface{}

	// Retries is the number of times this task has been retried.
	Retries int

	// Queue is the name of the queue this task was received from.
	Queue string
}

// ResultPayload contains the result of a task execution.
type ResultPayload struct {
	// TaskID is the unique identifier of the task that was executed.
	TaskID string

	// Status is the status of the task execution (success, error, etc.).
	Status string

	// Result is the result of the task execution (if successful).
	Result interface{}

	// Error is the error message (if the task execution failed).
	Error string

	// Traceback is the traceback of the error (if the task execution failed).
	Traceback string

	// Runtime is the duration of the task execution in seconds.
	Runtime float64

	// MemoryUsed is the amount of memory used by the task execution in bytes.
	MemoryUsed int64
}

// Protocol defines the interface for IPC protocols.
type Protocol interface {
	// EncodeTask encodes a task payload into a message that can be sent to a process.
	// For Celery v2 protocol, this should create a message with the Celery v2 format.
	EncodeTask(ctx context.Context, payload TaskPayload) ([]byte, error)

	// DecodeTask decodes a message into a task payload.
	// For Celery v2 protocol, this should parse a message with the Celery v2 format.
	DecodeTask(ctx context.Context, data []byte) (TaskPayload, error)

	// ParseCeleryMessage parses a Celery v2 protocol message.
	ParseCeleryMessage(ctx context.Context, data []byte) (*CeleryMessage, error)

	// TaskPayloadFromCeleryMessage extracts a TaskPayload from a Celery v2 protocol message.
	TaskPayloadFromCeleryMessage(ctx context.Context, message *CeleryMessage) (TaskPayload, error)

	// CeleryMessageFromTaskPayload creates a Celery v2 protocol message from a TaskPayload.
	CeleryMessageFromTaskPayload(ctx context.Context, payload TaskPayload) (*CeleryMessage, error)

	// EncodeResult encodes a result payload into a message that can be sent back to the worker.
	// This is used for internal communication between the Go worker and the Python process.
	EncodeResult(ctx context.Context, payload ResultPayload) ([]byte, error)

	// DecodeResult decodes a message into a result payload.
	// This is used for internal communication between the Go worker and the Python process.
	DecodeResult(ctx context.Context, data []byte) (ResultPayload, error)

	// EncodePing encodes a ping message.
	// This is used for internal communication between the Go worker and the Python process.
	EncodePing(ctx context.Context) ([]byte, error)

	// DecodePing decodes a ping message.
	// This is used for internal communication between the Go worker and the Python process.
	DecodePing(ctx context.Context, data []byte) error

	// EncodePong encodes a pong message.
	// This is used for internal communication between the Go worker and the Python process.
	EncodePong(ctx context.Context) ([]byte, error)

	// DecodePong decodes a pong message.
	// This is used for internal communication between the Go worker and the Python process.
	DecodePong(ctx context.Context, data []byte) error

	// EncodeStop encodes a stop message.
	// This is used for internal communication between the Go worker and the Python process.
	EncodeStop(ctx context.Context) ([]byte, error)

	// DecodeStop decodes a stop message.
	// This is used for internal communication between the Go worker and the Python process.
	DecodeStop(ctx context.Context, data []byte) error
}
