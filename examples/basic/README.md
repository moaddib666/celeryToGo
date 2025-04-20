# Basic Usage Example

This example demonstrates the simplest way to use CeleryToGo with minimal configuration.

## Prerequisites

- Go 1.23 or higher
- Python 3.6 or higher
- AWS credentials configured
- An SQS FIFO queue

## Configuration

The configuration in this example uses default values for most settings, only specifying the required queue URL.

```bash
./celeryToGo --queue-url=https://sqs.us-east-1.amazonaws.com/123456789012/my-queue.fifo
```

## Python Task Example

Here's a simple Celery task that can be executed by CeleryToGo:

```python
# tasks.py
from celery import Celery

app = Celery('tasks', broker='sqs://my-queue.fifo')

@app.task
def add(x, y):
    return x + y
```

## Running the Example

1. Build CeleryToGo:

```bash
make build
```

2. Start CeleryToGo:

```bash
./build/celeryToGo --queue-url=https://sqs.us-east-1.amazonaws.com/123456789012/my-queue.fifo
```

3. In another terminal, submit a task:

```python
# submit_task.py
from tasks import add

result = add.delay(4, 4)
print(f"Task ID: {result.id}")
```

4. CeleryToGo will execute the task and log the result.

## Configuration File

For convenience, you can also use the included `config.json` file:

```bash
./build/celeryToGo --config=examples/basic/config.json
```

## Explanation

In this basic example:

1. CeleryToGo connects to the specified SQS FIFO queue
2. It starts with default settings (10 concurrent tasks, 1-10 Python processes)
3. When a task is received, it's executed in a Python process
4. Results are sent back to the client

This configuration is suitable for simple workloads where the default settings are appropriate.