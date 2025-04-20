// Package celery provides an implementation of the protocol interface for Celery tasks.
package celery

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"celeryToGo/internal/protocol"
	"celeryToGo/internal/protocol/message"
)

// JSONProtocol is an implementation of the Protocol interface using JSON.
type JSONProtocol struct{}

// NewJSONProtocol creates a new JSONProtocol.
func NewJSONProtocol() *JSONProtocol {
	return &JSONProtocol{}
}

// ParseCeleryMessage parses a Celery v2 protocol message.
func (p *JSONProtocol) ParseCeleryMessage(ctx context.Context, data []byte) (*protocol.CeleryMessage, error) {
	var msg protocol.CeleryMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse Celery message: %w", err)
	}
	return &msg, nil
}

// TaskPayloadFromCeleryMessage extracts a TaskPayload from a Celery v2 protocol message.
func (p *JSONProtocol) TaskPayloadFromCeleryMessage(ctx context.Context, message *protocol.CeleryMessage) (protocol.TaskPayload, error) {
	// Extract task information from headers
	taskName, ok := message.Headers["task"].(string)
	if !ok {
		return protocol.TaskPayload{}, fmt.Errorf("task name not found in headers")
	}

	taskID, ok := message.Headers["id"].(string)
	if !ok {
		return protocol.TaskPayload{}, fmt.Errorf("task ID not found in headers")
	}

	// Extract retries from headers (default to 0 if not found)
	retries := 0
	if retriesVal, ok := message.Headers["retries"]; ok {
		switch v := retriesVal.(type) {
		case float64:
			retries = int(v)
		case int:
			retries = v
		case string:
			var err error
			retries, err = strconv.Atoi(v)
			if err != nil {
				retries = 0
			}
		}
	}

	// Extract queue from properties (if available)
	queue := ""
	if message.Properties != nil {
		if deliveryInfo, ok := message.Properties["delivery_info"].(map[string]interface{}); ok {
			if routingKey, ok := deliveryInfo["routing_key"].(string); ok {
				queue = routingKey
			}
		}
	}

	// Decode the body (base64-encoded JSON)
	bodyBytes, err := base64.StdEncoding.DecodeString(message.Body)
	if err != nil {
		return protocol.TaskPayload{}, fmt.Errorf("failed to decode body: %w", err)
	}

	// Parse the body as JSON
	var bodyData []interface{}
	if err := json.Unmarshal(bodyBytes, &bodyData); err != nil {
		return protocol.TaskPayload{}, fmt.Errorf("failed to parse body: %w", err)
	}

	// Extract args and kwargs from body
	// Celery v2 protocol body format: [args, kwargs, options]
	if len(bodyData) < 2 {
		return protocol.TaskPayload{}, fmt.Errorf("invalid body format")
	}

	// Extract args (first element in body)
	args, ok := bodyData[0].([]interface{})
	if !ok {
		args = []interface{}{}
	}

	// Extract kwargs (second element in body)
	kwargs, ok := bodyData[1].(map[string]interface{})
	if !ok {
		kwargs = map[string]interface{}{}
	}

	return protocol.TaskPayload{
		TaskName: taskName,
		TaskID:   taskID,
		Args:     args,
		Kwargs:   kwargs,
		Retries:  retries,
		Queue:    queue,
	}, nil
}

// CeleryMessageFromTaskPayload creates a Celery v2 protocol message from a TaskPayload.
func (p *JSONProtocol) CeleryMessageFromTaskPayload(ctx context.Context, payload protocol.TaskPayload) (*protocol.CeleryMessage, error) {
	// Create body data: [args, kwargs, options]
	bodyData := []interface{}{
		payload.Args,
		payload.Kwargs,
		map[string]interface{}{
			"callbacks": nil,
			"errbacks":  nil,
			"chain":     nil,
			"chord":     nil,
		},
	}

	// Encode body data as JSON
	bodyBytes, err := json.Marshal(bodyData)
	if err != nil {
		return nil, fmt.Errorf("failed to encode body: %w", err)
	}

	// Encode body as base64
	body := base64.StdEncoding.EncodeToString(bodyBytes)

	// Create headers
	headers := map[string]interface{}{
		"task":       payload.TaskName,
		"id":         payload.TaskID,
		"retries":    payload.Retries,
		"lang":       "py",
		"root_id":    payload.TaskID,
		"origin":     "celery-go-worker",
		"argsrepr":   fmt.Sprintf("%v", payload.Args),
		"kwargsrepr": fmt.Sprintf("%v", payload.Kwargs),
	}

	// Create properties
	properties := map[string]interface{}{
		"correlation_id": payload.TaskID,
		"delivery_mode":  2,
		"priority":       0,
		"body_encoding":  "base64",
	}

	if payload.Queue != "" {
		properties["delivery_info"] = map[string]interface{}{
			"exchange":    "",
			"routing_key": payload.Queue,
		}
	}

	return &protocol.CeleryMessage{
		Body:            body,
		ContentEncoding: "utf-8",
		ContentType:     "application/json",
		Headers:         headers,
		Properties:      properties,
	}, nil
}

// EncodeTask encodes a task payload into a message that can be sent to a process.
func (p *JSONProtocol) EncodeTask(ctx context.Context, payload protocol.TaskPayload) ([]byte, error) {
	// For internal communication with the Python process, we still use the old format
	// Create a task message from the payload
	msg := message.NewTaskMessage(payload)

	// Encode the message as JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to encode task message: %w", err)
	}

	return data, nil
}

// DecodeTask decodes a message into a task payload.
func (p *JSONProtocol) DecodeTask(ctx context.Context, data []byte) (protocol.TaskPayload, error) {
	// Try to parse as a Celery v2 protocol message first
	celeryMsg, err := p.ParseCeleryMessage(ctx, data)
	if err == nil {
		// Successfully parsed as a Celery v2 protocol message
		return p.TaskPayloadFromCeleryMessage(ctx, celeryMsg)
	}

	// If not a Celery v2 protocol message, try to parse as an internal message
	// Parse the message
	var baseMsg struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		return protocol.TaskPayload{}, fmt.Errorf("failed to parse message: %w", err)
	}

	// Check if the message is a task message
	if baseMsg.Type != protocol.InternalTaskMessage {
		return protocol.TaskPayload{}, fmt.Errorf("message is not a task message")
	}

	// Parse the full task message
	var taskMsg message.TaskMessage
	if err := json.Unmarshal(data, &taskMsg); err != nil {
		return protocol.TaskPayload{}, fmt.Errorf("failed to parse task message: %w", err)
	}

	// Convert the message to a payload
	return taskMsg.ToPayload(), nil
}

// EncodeResult encodes a result payload into a message that can be sent back to the worker.
func (p *JSONProtocol) EncodeResult(ctx context.Context, payload protocol.ResultPayload) ([]byte, error) {
	// Create a result message from the payload
	msg := message.NewResultMessage(payload)

	// Encode the message as JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to encode result message: %w", err)
	}

	return data, nil
}

// DecodeResult decodes a message into a result payload.
func (p *JSONProtocol) DecodeResult(ctx context.Context, data []byte) (protocol.ResultPayload, error) {
	// Parse the message
	var baseMsg struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		return protocol.ResultPayload{}, fmt.Errorf("failed to parse message: %w", err)
	}

	// Check if the message is a result message
	if baseMsg.Type != protocol.InternalResultMessage {
		return protocol.ResultPayload{}, fmt.Errorf("message is not a result message")
	}

	// Parse the full result message
	var resultMsg message.ResultMessage
	if err := json.Unmarshal(data, &resultMsg); err != nil {
		return protocol.ResultPayload{}, fmt.Errorf("failed to parse result message: %w", err)
	}

	// Convert the message to a payload
	return resultMsg.ToPayload(), nil
}

// EncodePing encodes a ping message.
func (p *JSONProtocol) EncodePing(ctx context.Context) ([]byte, error) {
	// Create a ping message
	msg := message.NewPingMessage()

	// Encode the message as JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to encode ping message: %w", err)
	}

	return data, nil
}

// DecodePing decodes a ping message.
func (p *JSONProtocol) DecodePing(ctx context.Context, data []byte) error {
	// Parse the message
	var baseMsg struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Check if the message is a ping message
	if baseMsg.Type != protocol.InternalPingMessage {
		return fmt.Errorf("message is not a ping message")
	}

	return nil
}

// EncodePong encodes a pong message.
func (p *JSONProtocol) EncodePong(ctx context.Context) ([]byte, error) {
	// Create a pong message
	// In a real implementation, we would use the ID of the ping message
	msg := message.NewPongMessage("")

	// Encode the message as JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to encode pong message: %w", err)
	}

	return data, nil
}

// DecodePong decodes a pong message.
func (p *JSONProtocol) DecodePong(ctx context.Context, data []byte) error {
	// Parse the message
	var baseMsg struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Check if the message is a pong message
	if baseMsg.Type != protocol.InternalPongMessage {
		return fmt.Errorf("message is not a pong message")
	}

	return nil
}

// EncodeStop encodes a stop message.
func (p *JSONProtocol) EncodeStop(ctx context.Context) ([]byte, error) {
	// Create a stop message
	msg := message.NewStopMessage(true)

	// Encode the message as JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to encode stop message: %w", err)
	}

	return data, nil
}

// DecodeStop decodes a stop message.
func (p *JSONProtocol) DecodeStop(ctx context.Context, data []byte) error {
	// Parse the message
	var baseMsg struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Check if the message is a stop message
	if baseMsg.Type != protocol.InternalStopMessage {
		return fmt.Errorf("message is not a stop message")
	}

	return nil
}
