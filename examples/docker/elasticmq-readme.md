# ElasticMQ Configuration for Local SQS Testing

This document explains how to configure ElasticMQ for local SQS testing with CeleryToGo.

## What is ElasticMQ?

ElasticMQ is a message queue system that implements the Amazon SQS API, making it perfect for local development and testing without requiring an actual AWS account or SQS queues.

## Docker Configuration

To use ElasticMQ with the Docker setup:

1. Uncomment the ElasticMQ service in `docker-compose.yml`
2. Update the queue URL in `config.json` or `.env` to point to ElasticMQ:
   ```
   QUEUE_URL=http://elasticmq:9324/queue/test.fifo
   ```

## ElasticMQ Configuration

Create a file named `elasticmq.conf` in the same directory as your `docker-compose.yml` with the following content:

```hocon
include classpath("application.conf")

node-address {
    protocol = http
    host = "0.0.0.0"
    port = 9324
    context-path = ""
}

rest-sqs {
    enabled = true
    bind-port = 9324
    bind-hostname = "0.0.0.0"
    sqs-limits = strict
}

queues {
    test.fifo {
        defaultVisibilityTimeout = 60 seconds
        delay = 0 seconds
        receiveMessageWait = 20 seconds
        fifo = true
        contentBasedDeduplication = false
    }
    
    high-priority.fifo {
        defaultVisibilityTimeout = 60 seconds
        delay = 0 seconds
        receiveMessageWait = 20 seconds
        fifo = true
        contentBasedDeduplication = false
    }
    
    low-priority.fifo {
        defaultVisibilityTimeout = 60 seconds
        delay = 0 seconds
        receiveMessageWait = 20 seconds
        fifo = true
        contentBasedDeduplication = false
    }
}

management-api {
    enabled = true
    bind-port = 9325
    bind-hostname = "0.0.0.0"
}
```

This configuration creates three FIFO queues:
- `test.fifo`: A general-purpose test queue
- `high-priority.fifo`: A queue for high-priority tasks
- `low-priority.fifo`: A queue for low-priority tasks

## Using ElasticMQ with CeleryToGo

When using ElasticMQ, you need to:

1. Set the queue URL to point to ElasticMQ:
   ```
   http://elasticmq:9324/queue/test.fifo
   ```

2. Set the endpoint to ElasticMQ:
   ```
   --endpoint=http://elasticmq:9324
   ```

3. Make sure your Python code is configured to use the same queue URL.

## Testing with ElasticMQ

You can test the ElasticMQ setup using the AWS CLI:

```bash
# List queues
aws --endpoint-url http://localhost:9324 sqs list-queues

# Send a message to a FIFO queue
aws --endpoint-url http://localhost:9324 sqs send-message \
  --queue-url http://localhost:9324/queue/test.fifo \
  --message-body '{"task": "app.tasks.add", "args": [1, 2]}' \
  --message-group-id "task-group-1" \
  --message-deduplication-id "unique-$(date +%s)"

# Receive messages
aws --endpoint-url http://localhost:9324 sqs receive-message \
  --queue-url http://localhost:9324/queue/test.fifo
```

## Management UI

ElasticMQ doesn't come with a management UI by default, but you can use tools like:

- [SQS Web Console](https://github.com/kobim/sqs-insight)
- [AWS Console for Local Development](https://github.com/mhart/aws-console)

Or access the management API directly at `http://localhost:9325`.