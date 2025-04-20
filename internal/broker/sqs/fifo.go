// Package sqs provides an implementation of the broker interface for AWS SQS FIFO queues.
package sqs

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"celeryToGo/internal/broker"
)

// Errors returned by the SQSFIFOBroker.
var (
	ErrQueueURLEmpty        = errors.New("queue URL is empty")
	ErrMessageGroupIDEmpty  = errors.New("message group ID is empty for FIFO queue")
	ErrDeduplicationIDEmpty = errors.New("deduplication ID is empty for FIFO queue")
)

// MessageGroupQueue represents a queue of messages for a specific message group.
type MessageGroupQueue struct {
	// GroupID is the message group ID.
	GroupID string

	// Processing indicates whether a message from this group is currently being processed.
	Processing bool

	// LastAccess is the time when this group was last accessed.
	LastAccess time.Time

	// Tasks is a queue of messages for this group.
	// Messages are processed in FIFO order.
	Tasks []broker.Message
}

// SQSFIFOBroker is an implementation of the broker interface for AWS SQS FIFO queues.
type SQSFIFOBroker struct {
	// client is the AWS SQS client.
	client *sqs.Client

	// queueURL is the URL of the SQS queue.
	queueURL string

	// config contains configuration for the broker.
	config broker.Config

	// activeGroups is a map of message group IDs to their processing state.
	activeGroups map[string]*MessageGroupQueue

	// groupsMutex protects access to activeGroups.
	groupsMutex sync.RWMutex

	// receiveParams are the parameters for receiving messages.
	receiveParams *sqs.ReceiveMessageInput

	// logger is the logger for the broker.
	logger *logrus.Entry
}

// NewSQSFIFOBroker creates a new SQSFIFOBroker.
func NewSQSFIFOBroker(client *sqs.Client, config broker.Config) (*SQSFIFOBroker, error) {
	if config.QueueURL == "" {
		return nil, ErrQueueURLEmpty
	}

	// Ensure the queue URL ends with .fifo
	if len(config.QueueURL) < 5 || config.QueueURL[len(config.QueueURL)-5:] != ".fifo" {
		return nil, fmt.Errorf("queue URL does not end with .fifo: %s", config.QueueURL)
	}

	// Set default values if not provided
	if config.MaxMessages <= 0 {
		config.MaxMessages = 10
	}
	if config.VisibilityTimeout <= 0 {
		config.VisibilityTimeout = 30
	}
	if config.WaitTimeSeconds <= 0 {
		config.WaitTimeSeconds = 20
	}

	// Set default values for new configuration options
	if config.FetchMode == "" {
		config.FetchMode = "batch" // Default to batch mode
	} else if config.FetchMode != "single" && config.FetchMode != "batch" {
		return nil, fmt.Errorf("invalid fetch mode: %s, must be 'single' or 'batch'", config.FetchMode)
	}

	if config.BatchSize <= 0 {
		config.BatchSize = 10 // Default batch size
	} else if config.BatchSize < 2 {
		config.BatchSize = 2 // Minimum batch size
	} else if config.BatchSize > 10 {
		config.BatchSize = 10 // Maximum batch size
	}

	if config.MaxPrefetch <= 0 {
		config.MaxPrefetch = 50 // Default max prefetch
	}

	// Create receive message parameters
	receiveParams := &sqs.ReceiveMessageInput{
		QueueUrl:              aws.String(config.QueueURL),
		MaxNumberOfMessages:   int32(config.MaxMessages),
		VisibilityTimeout:     int32(config.VisibilityTimeout),
		WaitTimeSeconds:       int32(config.WaitTimeSeconds),
		MessageAttributeNames: []string{"All"},
		AttributeNames:        []types.QueueAttributeName{types.QueueAttributeNameAll},
	}

	logger := logrus.WithFields(logrus.Fields{
		"component":    "sqs_fifo_broker",
		"queue_url":    config.QueueURL,
		"fetch_mode":   config.FetchMode,
		"batch_size":   config.BatchSize,
		"max_prefetch": config.MaxPrefetch,
	})

	return &SQSFIFOBroker{
		client:        client,
		queueURL:      config.QueueURL,
		config:        config,
		activeGroups:  make(map[string]*MessageGroupQueue),
		receiveParams: receiveParams,
		logger:        logger,
	}, nil
}

// Connect establishes a connection to the broker.
func (b *SQSFIFOBroker) Connect(ctx context.Context) error {
	b.logger.Info("Connecting to SQS FIFO queue")

	// SQS doesn't require an explicit connection, but we can validate the queue
	_, err := b.client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(b.queueURL),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameFifoQueue},
	})
	if err != nil {
		b.logger.WithError(err).Error("Failed to connect to SQS queue")
		return fmt.Errorf("failed to connect to SQS queue: %w", err)
	}

	b.logger.Info("Connected to SQS FIFO queue")
	return nil
}

// Close closes the connection to the broker.
func (b *SQSFIFOBroker) Close(ctx context.Context) error {
	b.logger.Info("Closing connection to SQS FIFO queue")
	// SQS doesn't require an explicit close
	return nil
}

// ReceiveMessages receives up to maxMessages from the broker.
func (b *SQSFIFOBroker) ReceiveMessages(ctx context.Context, maxMessages int) ([]broker.Message, error) {
	// Limit maxMessages to MaxPrefetch
	maxMessages = min(maxMessages, b.config.MaxPrefetch)

	// Check if we have any messages in our internal queues
	internalMessages := b.getMessagesFromInternalQueues(maxMessages)
	if len(internalMessages) > 0 {
		return internalMessages, nil
	}

	// If we're in single mode, receive one message at a time
	if b.config.FetchMode == "single" {
		return b.receiveSingleMessage(ctx)
	}

	// Otherwise, we're in batch mode
	return b.receiveBatchMessages(ctx, maxMessages)
}

// receiveSingleMessage receives a single message from SQS.
func (b *SQSFIFOBroker) receiveSingleMessage(ctx context.Context) ([]broker.Message, error) {
	// Get the list of message groups that are not being processed
	availableGroups := b.getAvailableGroups()

	// Create a copy of the receive parameters
	receiveParams := *b.receiveParams
	receiveParams.MaxNumberOfMessages = 1 // Only receive one message

	// Add a unique request attempt ID
	receiveParams.ReceiveRequestAttemptId = aws.String(fmt.Sprintf("req-%s", uuid.New().String()))

	b.logger.WithFields(logrus.Fields{
		"fetch_mode":       "single",
		"available_groups": len(availableGroups),
	}).Debug("Receiving single message from SQS")

	// Receive messages from SQS
	result, err := b.client.ReceiveMessage(ctx, &receiveParams)
	if err != nil {
		b.logger.WithError(err).Error("Failed to receive message from SQS")
		return nil, fmt.Errorf("failed to receive message from SQS: %w", err)
	}

	if len(result.Messages) == 0 {
		return []broker.Message{}, nil
	}

	b.logger.WithField("message_count", len(result.Messages)).Debug("Received message from SQS")

	// Convert SQS message to broker message
	sqsMsg := result.Messages[0]

	// Extract message group ID
	groupID, ok := sqsMsg.Attributes["MessageGroupId"]
	if !ok {
		b.logger.WithField("message_id", *sqsMsg.MessageId).Warn("Received message without group ID from FIFO queue")
		return []broker.Message{}, nil
	}

	// Mark this group as being processed
	b.markGroupAsProcessing(groupID)

	// Convert SQS message to broker message
	// Decode the base64 encoded message body
	decodedBody, err := base64.StdEncoding.DecodeString(*sqsMsg.Body)
	if err != nil {
		b.logger.WithError(err).WithField("message_id", *sqsMsg.MessageId).Error("Failed to decode message body")
		return nil, fmt.Errorf("failed to decode message body: %w", err)
	}

	msg := broker.Message{
		ID:                     *sqsMsg.MessageId,
		Body:                   decodedBody,
		Attributes:             make(map[string]string),
		MessageGroupID:         groupID,
		MessageDeduplicationID: sqsMsg.Attributes["MessageDeduplicationId"],
		ReceiptHandle:          *sqsMsg.ReceiptHandle,
	}

	// Copy attributes
	for k, v := range sqsMsg.Attributes {
		msg.Attributes[k] = v
	}

	// Copy message attributes
	for k, v := range sqsMsg.MessageAttributes {
		if v.StringValue != nil {
			msg.Attributes[k] = *v.StringValue
		}
	}

	b.logger.WithFields(logrus.Fields{
		"message_id":       msg.ID,
		"message_group_id": msg.MessageGroupID,
	}).Debug("Processed message from SQS")

	return []broker.Message{msg}, nil
}

// receiveBatchMessages receives multiple messages from SQS in batch mode.
func (b *SQSFIFOBroker) receiveBatchMessages(ctx context.Context, maxMessages int) ([]broker.Message, error) {
	// Get the list of message groups that are not being processed
	availableGroups := b.getAvailableGroups()

	// Create a copy of the receive parameters
	receiveParams := *b.receiveParams

	// Determine how many messages to receive
	batchSize := min(b.config.BatchSize, maxMessages)

	// If we have available groups, limit the number of messages to receive
	if len(availableGroups) > 0 {
		receiveParams.MaxNumberOfMessages = int32(min(batchSize, len(availableGroups)))
	} else {
		// If no groups are available, still try to receive messages to discover new groups
		receiveParams.MaxNumberOfMessages = int32(batchSize)
	}

	// Add a unique request attempt ID
	receiveParams.ReceiveRequestAttemptId = aws.String(fmt.Sprintf("req-%s", uuid.New().String()))

	b.logger.WithFields(logrus.Fields{
		"fetch_mode":       "batch",
		"batch_size":       batchSize,
		"available_groups": len(availableGroups),
	}).Debug("Receiving batch messages from SQS")

	// Receive messages from SQS
	result, err := b.client.ReceiveMessage(ctx, &receiveParams)
	if err != nil {
		b.logger.WithError(err).Error("Failed to receive messages from SQS")
		return nil, fmt.Errorf("failed to receive messages from SQS: %w", err)
	}

	b.logger.WithField("message_count", len(result.Messages)).Debug("Received messages from SQS")

	// Group messages by their message group ID
	messagesByGroup := make(map[string][]broker.Message)

	// Convert SQS messages to broker messages and group them
	for _, sqsMsg := range result.Messages {
		// Extract message group ID
		groupID, ok := sqsMsg.Attributes["MessageGroupId"]
		if !ok {
			b.logger.WithField("message_id", *sqsMsg.MessageId).Warn("Received message without group ID from FIFO queue")
			continue
		}

		// Convert SQS message to broker message
		// Decode the base64 encoded message body
		decodedBody, err := base64.StdEncoding.DecodeString(*sqsMsg.Body)
		if err != nil {
			b.logger.WithError(err).WithField("message_id", *sqsMsg.MessageId).Error("Failed to decode message body")
			continue
		}

		msg := broker.Message{
			ID:                     *sqsMsg.MessageId,
			Body:                   decodedBody,
			Attributes:             make(map[string]string),
			MessageGroupID:         groupID,
			MessageDeduplicationID: sqsMsg.Attributes["MessageDeduplicationId"],
			ReceiptHandle:          *sqsMsg.ReceiptHandle,
		}

		// Copy attributes
		for k, v := range sqsMsg.Attributes {
			msg.Attributes[k] = v
		}

		// Copy message attributes
		for k, v := range sqsMsg.MessageAttributes {
			if v.StringValue != nil {
				msg.Attributes[k] = *v.StringValue
			}
		}

		// Add message to its group
		messagesByGroup[groupID] = append(messagesByGroup[groupID], msg)

		b.logger.WithFields(logrus.Fields{
			"message_id":       msg.ID,
			"message_group_id": msg.MessageGroupID,
		}).Debug("Processed message from SQS")
	}

	// Process each group
	messages := make([]broker.Message, 0, len(result.Messages))
	for groupID, groupMessages := range messagesByGroup {
		// Mark this group as being processed and add messages to its queue
		b.addMessagesToGroup(groupID, groupMessages)

		// Take the first message from this group
		if len(groupMessages) > 0 {
			messages = append(messages, groupMessages[0])
		}
	}

	return messages, nil
}

// getMessagesFromInternalQueues gets messages from internal queues.
func (b *SQSFIFOBroker) getMessagesFromInternalQueues(maxMessages int) []broker.Message {
	b.groupsMutex.Lock()
	defer b.groupsMutex.Unlock()

	messages := make([]broker.Message, 0, maxMessages)

	// Iterate through all groups
	for _, group := range b.activeGroups {
		// Skip groups that are being processed or have no messages
		if group.Processing || len(group.Tasks) == 0 {
			continue
		}

		// Take the first message from this group
		message := group.Tasks[0]
		group.Tasks = group.Tasks[1:] // Remove the message from the queue

		// Mark the group as processing
		group.Processing = true
		group.LastAccess = time.Now()

		messages = append(messages, message)

		// Stop if we have enough messages
		if len(messages) >= maxMessages {
			break
		}
	}

	return messages
}

// addMessagesToGroup adds messages to a group's queue and marks it as processing.
func (b *SQSFIFOBroker) addMessagesToGroup(groupID string, messages []broker.Message) {
	b.groupsMutex.Lock()
	defer b.groupsMutex.Unlock()

	group, exists := b.activeGroups[groupID]
	if !exists {
		group = &MessageGroupQueue{
			GroupID: groupID,
			Tasks:   make([]broker.Message, 0, len(messages)),
		}
		b.activeGroups[groupID] = group
	}

	// If this is the first message, mark the group as processing
	if len(group.Tasks) == 0 && len(messages) > 0 {
		group.Processing = true
	}

	// Add all messages except the first one to the queue
	if len(messages) > 1 {
		group.Tasks = append(group.Tasks, messages[1:]...)
	}

	group.LastAccess = time.Now()

	b.logger.WithFields(logrus.Fields{
		"group_id":      groupID,
		"message_count": len(messages),
		"queue_size":    len(group.Tasks),
	}).Debug("Added messages to group queue")
}

// AcknowledgeMessage acknowledges a message, removing it from the queue.
func (b *SQSFIFOBroker) AcknowledgeMessage(ctx context.Context, message broker.Message) error {
	b.logger.WithFields(logrus.Fields{
		"message_id":       message.ID,
		"message_group_id": message.MessageGroupID,
	}).Debug("Acknowledging message")

	// Delete the message from SQS
	_, err := b.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(b.queueURL),
		ReceiptHandle: aws.String(message.ReceiptHandle),
	})
	if err != nil {
		b.logger.WithError(err).WithField("message_id", message.ID).Error("Failed to delete message from SQS")
		return fmt.Errorf("failed to delete message from SQS: %w", err)
	}

	// Mark the message group as available for processing
	// This will check if there are more messages in the queue and handle them appropriately
	b.markGroupAsAvailable(message.MessageGroupID)

	b.logger.WithFields(logrus.Fields{
		"message_id":       message.ID,
		"message_group_id": message.MessageGroupID,
	}).Debug("Message acknowledged")

	return nil
}

// PublishMessage publishes a message to the broker.
func (b *SQSFIFOBroker) PublishMessage(ctx context.Context, message broker.Message) error {
	// Validate message group ID
	if message.MessageGroupID == "" {
		return ErrMessageGroupIDEmpty
	}

	// Validate deduplication ID
	if message.MessageDeduplicationID == "" {
		return ErrDeduplicationIDEmpty
	}

	b.logger.WithFields(logrus.Fields{
		"message_group_id": message.MessageGroupID,
	}).Debug("Publishing message to SQS")

	// Create message attributes
	messageAttributes := make(map[string]types.MessageAttributeValue)
	for k, v := range message.Attributes {
		messageAttributes[k] = types.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(v),
		}
	}

	// Send message to SQS
	result, err := b.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:               aws.String(b.queueURL),
		MessageBody:            aws.String(string(message.Body)),
		MessageGroupId:         aws.String(message.MessageGroupID),
		MessageDeduplicationId: aws.String(message.MessageDeduplicationID),
		MessageAttributes:      messageAttributes,
	})
	if err != nil {
		b.logger.WithError(err).Error("Failed to send message to SQS")
		return fmt.Errorf("failed to send message to SQS: %w", err)
	}

	b.logger.WithFields(logrus.Fields{
		"message_id":       *result.MessageId,
		"message_group_id": message.MessageGroupID,
	}).Debug("Message published to SQS")

	return nil
}

// ExtendVisibilityTimeout extends the visibility timeout for a message.
func (b *SQSFIFOBroker) ExtendVisibilityTimeout(ctx context.Context, message broker.Message, seconds int) error {
	b.logger.WithFields(logrus.Fields{
		"message_id":       message.ID,
		"message_group_id": message.MessageGroupID,
		"seconds":          seconds,
	}).Debug("Extending message visibility timeout")

	_, err := b.client.ChangeMessageVisibility(ctx, &sqs.ChangeMessageVisibilityInput{
		QueueUrl:          aws.String(b.queueURL),
		ReceiptHandle:     aws.String(message.ReceiptHandle),
		VisibilityTimeout: int32(seconds),
	})
	if err != nil {
		b.logger.WithError(err).WithField("message_id", message.ID).Error("Failed to extend message visibility timeout")
		return fmt.Errorf("failed to extend message visibility timeout: %w", err)
	}

	b.logger.WithFields(logrus.Fields{
		"message_id":       message.ID,
		"message_group_id": message.MessageGroupID,
		"seconds":          seconds,
	}).Debug("Message visibility timeout extended")

	return nil
}

// getAvailableGroups returns a list of message groups that are not being processed.
func (b *SQSFIFOBroker) getAvailableGroups() []string {
	b.groupsMutex.RLock()
	defer b.groupsMutex.RUnlock()

	availableGroups := make([]string, 0)

	// First, add any known groups that are not being processed
	for groupID, group := range b.activeGroups {
		if !group.Processing {
			availableGroups = append(availableGroups, groupID)
		}
	}

	// If we have no available groups, we need to check if there are any new groups in the queue
	// by receiving a message with a very short visibility timeout
	if len(availableGroups) == 0 {
		// Create a context with a short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Create a copy of the receive parameters with a short visibility timeout
		receiveParams := *b.receiveParams
		receiveParams.MaxNumberOfMessages = 1
		receiveParams.VisibilityTimeout = 1 // 1 second visibility timeout
		receiveParams.ReceiveRequestAttemptId = aws.String(fmt.Sprintf("probe-%s", uuid.New().String()))

		// Receive a message to probe for available groups
		result, err := b.client.ReceiveMessage(ctx, &receiveParams)
		if err != nil {
			b.logger.WithError(err).Error("Failed to probe for available message groups")
			return availableGroups
		}

		// If we received a message, extract the group ID and change the visibility timeout back
		for _, sqsMsg := range result.Messages {
			groupID, ok := sqsMsg.Attributes["MessageGroupId"]
			if !ok {
				continue
			}

			// Check if this group is already being processed
			if group, exists := b.activeGroups[groupID]; exists && group.Processing {
				continue
			}

			// Add this group to the available groups
			availableGroups = append(availableGroups, groupID)

			// Change the visibility timeout back to the original value
			_, err := b.client.ChangeMessageVisibility(ctx, &sqs.ChangeMessageVisibilityInput{
				QueueUrl:          aws.String(b.queueURL),
				ReceiptHandle:     sqsMsg.ReceiptHandle,
				VisibilityTimeout: b.receiveParams.VisibilityTimeout,
			})
			if err != nil {
				b.logger.WithError(err).WithField("group_id", groupID).Error("Failed to change message visibility timeout")
			}

			// Convert SQS message to broker message
			// Decode the base64 encoded message body
			decodedBody, err := base64.StdEncoding.DecodeString(*sqsMsg.Body)
			if err != nil {
				b.logger.WithError(err).WithField("message_id", *sqsMsg.MessageId).Error("Failed to decode message body")
				continue
			}

			msg := broker.Message{
				ID:                     *sqsMsg.MessageId,
				Body:                   decodedBody,
				Attributes:             make(map[string]string),
				MessageGroupID:         groupID,
				MessageDeduplicationID: sqsMsg.Attributes["MessageDeduplicationId"],
				ReceiptHandle:          *sqsMsg.ReceiptHandle,
			}

			// Copy attributes
			for k, v := range sqsMsg.Attributes {
				msg.Attributes[k] = v
			}

			// Copy message attributes
			for k, v := range sqsMsg.MessageAttributes {
				if v.StringValue != nil {
					msg.Attributes[k] = *v.StringValue
				}
			}

			// Add the message to the group's queue
			if group, exists := b.activeGroups[groupID]; exists {
				group.Tasks = append(group.Tasks, msg)
			} else {
				// Create a new group with this message
				b.activeGroups[groupID] = &MessageGroupQueue{
					GroupID:    groupID,
					Processing: false,
					LastAccess: time.Now(),
					Tasks:      []broker.Message{msg},
				}
			}
		}
	}

	return availableGroups
}

// markGroupAsProcessing marks a message group as being processed.
func (b *SQSFIFOBroker) markGroupAsProcessing(groupID string) {
	b.groupsMutex.Lock()
	defer b.groupsMutex.Unlock()

	group, exists := b.activeGroups[groupID]
	if !exists {
		group = &MessageGroupQueue{
			GroupID: groupID,
			Tasks:   make([]broker.Message, 0),
		}
		b.activeGroups[groupID] = group
	}

	group.Processing = true
	group.LastAccess = time.Now()

	b.logger.WithField("group_id", groupID).Debug("Marked message group as processing")
}

// markGroupAsAvailable marks a message group as available for processing.
func (b *SQSFIFOBroker) markGroupAsAvailable(groupID string) {
	b.groupsMutex.Lock()
	defer b.groupsMutex.Unlock()

	group, exists := b.activeGroups[groupID]
	if !exists {
		return
	}

	// Check if there are any messages in the queue
	if len(group.Tasks) > 0 {
		// There are more messages in the queue, so we keep the group as processing
		// and return the next message in the next call to ReceiveMessages
		b.logger.WithFields(logrus.Fields{
			"group_id":   groupID,
			"queue_size": len(group.Tasks),
		}).Debug("Group has more messages in queue, keeping as processing")
	} else {
		// No more messages in the queue, mark the group as available
		group.Processing = false
		b.logger.WithField("group_id", groupID).Debug("Marked message group as available")
	}

	group.LastAccess = time.Now()
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
