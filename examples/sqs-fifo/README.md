# SQS FIFO Queue Configuration Example

This example demonstrates how to configure CeleryToGo for optimal performance with AWS SQS FIFO queues, focusing on proper message group handling to maintain ordering guarantees.

## Prerequisites

- Go 1.23 or higher
- Python 3.6 or higher
- AWS credentials configured
- An SQS FIFO queue

## SQS FIFO Queue Concepts

SQS FIFO (First-In-First-Out) queues have several important characteristics:

1. **Message Ordering**: Messages are delivered in the exact order they were sent.
2. **Message Groups**: Messages with the same group ID are processed in order.
3. **Deduplication**: Each message is delivered exactly once.

## Configuration

The configuration in this example is optimized for SQS FIFO queues:

```bash
./celeryToGo \
  --queue-url=https://sqs.us-east-1.amazonaws.com/123456789012/my-queue.fifo \
  --region=us-east-1 \
  --max-messages=10 \
  --visibility-timeout=60 \
  --wait-time=20
```

## Message Group Handling

CeleryToGo handles message groups properly by:

1. Tracking active message groups
2. Processing only one message per group at a time
3. Acknowledging messages only after successful processing
4. Maintaining order within each group

## Python Task Example with Message Groups

Here's an example of submitting tasks with specific message groups:

```python
# tasks.py
from celery import Celery
import uuid

app = Celery('tasks', broker='sqs://my-queue.fifo')

@app.task
def process_order(order_id, items):
    # Process the order
    return f"Order {order_id} processed with {len(items)} items"

# Helper function to submit tasks with a specific message group
def submit_order_task(order_id, customer_id, items):
    # Use customer_id as the message group to ensure all orders for the same customer
    # are processed in order
    headers = {
        'kwargs': {
            'message_group_id': f'customer-{customer_id}',
            # Generate a unique deduplication ID
            'message_deduplication_id': f'order-{order_id}-{uuid.uuid4()}'
        }
    }
    
    return process_order.apply_async(
        args=[order_id, items],
        headers=headers
    )
```

## Running the Example

1. Build CeleryToGo:

```bash
make build
```

2. Start CeleryToGo with the FIFO configuration:

```bash
./build/celeryToGo --config=examples/sqs-fifo/config.json
```

3. In another terminal, submit tasks with message groups:

```python
# submit_tasks.py
from tasks import submit_order_task

# These will be processed in order because they have the same customer ID
submit_order_task("order-001", "customer-123", ["item1", "item2"])
submit_order_task("order-002", "customer-123", ["item3"])
submit_order_task("order-003", "customer-123", ["item4", "item5"])

# These will be processed in order for customer-456, but can be processed
# in parallel with the customer-123 orders
submit_order_task("order-004", "customer-456", ["item6"])
submit_order_task("order-005", "customer-456", ["item7", "item8"])
```

## Explanation

In this SQS FIFO example:

1. CeleryToGo connects to the specified SQS FIFO queue
2. It tracks message groups to ensure ordering
3. Tasks with the same message group ID are processed in order
4. Tasks with different message group IDs can be processed in parallel
5. Deduplication IDs ensure each task is processed exactly once

This configuration is suitable for workloads where message ordering is critical, such as financial transactions, order processing, or any sequential workflow.

## Best Practices

1. **Choose Message Groups Carefully**: Use a business entity (like customer ID, order ID) that requires sequential processing.
2. **Generate Unique Deduplication IDs**: Ensure each message has a unique deduplication ID.
3. **Set Appropriate Visibility Timeout**: Set it high enough to allow task completion.
4. **Monitor Group Processing**: Watch for "stuck" message groups that could block processing.