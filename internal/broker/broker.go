// Package broker provides interfaces and implementations for message brokers.
package broker

import (
	"context"
)

// Message represents a message from a broker.
type Message struct {
	// ID is the unique identifier for the message.
	ID string

	// Body is the message content.
	Body []byte

	// Attributes contains broker-specific message attributes.
	Attributes map[string]string

	// MessageGroupID is used for FIFO queues to specify the group that this message belongs to.
	MessageGroupID string

	// MessageDeduplicationID is used for FIFO queues to detect and eliminate duplicate messages.
	MessageDeduplicationID string

	// ReceiptHandle is a token used to acknowledge the message.
	ReceiptHandle string
}

// Broker defines the interface for message brokers.
type Broker interface {
	// Connect establishes a connection to the broker.
	Connect(ctx context.Context) error

	// Close closes the connection to the broker.
	Close(ctx context.Context) error

	// ReceiveMessages receives up to maxMessages from the broker.
	// It returns the received messages and any error encountered.
	ReceiveMessages(ctx context.Context, maxMessages int) ([]Message, error)

	// AcknowledgeMessage acknowledges a message, removing it from the queue.
	AcknowledgeMessage(ctx context.Context, message Message) error

	// PublishMessage publishes a message to the broker.
	PublishMessage(ctx context.Context, message Message) error

	// ExtendVisibilityTimeout extends the visibility timeout for a message.
	ExtendVisibilityTimeout(ctx context.Context, message Message, seconds int) error
}

// Config contains configuration for a broker.
type Config struct {
	// QueueURL is the URL of the queue.
	QueueURL string

	// Region is the AWS region for the queue.
	Region string

	// Endpoint is the endpoint URL for the queue service.
	Endpoint string

	// MaxMessages is the maximum number of messages to receive in a single call.
	MaxMessages int

	// VisibilityTimeout is the duration (in seconds) that the received messages are hidden from subsequent retrieve requests.
	VisibilityTimeout int

	// WaitTimeSeconds is the duration (in seconds) for which the call waits for a message to arrive in the queue before returning.
	WaitTimeSeconds int

	// FetchMode determines how messages are fetched from the queue.
	// Valid values are "single" (one message at a time) and "batch" (multiple messages).
	FetchMode string

	// BatchSize is the maximum number of messages to fetch in batch mode (2-10).
	// Only used when FetchMode is "batch".
	BatchSize int

	// MaxPrefetch is the maximum number of messages to prefetch.
	// Default is 50.
	MaxPrefetch int
}
