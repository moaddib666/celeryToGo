# Custom Process Pool Configuration Example

This example demonstrates how to configure the Python process pool in CeleryToGo for different workloads and resource constraints.

## Prerequisites

- Go 1.23 or higher
- Python 3.6 or higher
- AWS credentials configured
- An SQS queue

## Process Pool Concepts

CeleryToGo manages a pool of Python processes to execute Celery tasks. Key concepts include:

1. **Process Lifecycle**: Processes are created, used to execute tasks, and eventually recycled.
2. **Process Recycling**: Processes are recycled based on age, idle time, or number of tasks processed.
3. **Resource Management**: The pool balances between having enough processes for throughput and avoiding excessive resource usage.

## Configuration Options

The process pool can be configured with the following options:

```bash
./celeryToGo \
  --queue-url=https://sqs.us-east-1.amazonaws.com/123456789012/my-queue.fifo \
  --min-processes=10 \
  --max-processes=50 \
  --max-tasks-per-process=200 \
  --max-idle-time=10m \
  --max-process-age=2h \
  --process-startup-timeout=45s \
  --process-shutdown-timeout=30s \
  --python-path=/usr/local/bin/python3.9 \
  --worker-script-path=/path/to/custom/worker_script.py
```

## Configuration Scenarios

### High-Throughput Workload

For workloads with many small, quick tasks:

```json
{
  "min_processes": 20,
  "max_processes": 100,
  "max_tasks_per_process": 500,
  "max_idle_time": "5m",
  "max_process_age": "30m"
}
```

### Memory-Intensive Workload

For tasks that use a lot of memory:

```json
{
  "min_processes": 5,
  "max_processes": 20,
  "max_tasks_per_process": 50,
  "max_idle_time": "10m",
  "max_process_age": "1h"
}
```

### Long-Running Tasks

For tasks that take a long time to complete:

```json
{
  "min_processes": 10,
  "max_processes": 30,
  "max_tasks_per_process": 20,
  "max_idle_time": "15m",
  "max_process_age": "2h",
  "task_timeout": "30m"
}
```

## Custom Python Environment

You can specify a custom Python executable and environment variables:

```bash
DJANGO_SETTINGS_MODULE=myproject.settings \
PYTHONPATH=/path/to/project \
./celeryToGo \
  --queue-url=https://sqs.us-east-1.amazonaws.com/123456789012/my-queue.fifo \
  --python-path=/path/to/venv/bin/python \
  --worker-script-path=/path/to/custom/worker_script.py
```

## Running the Example

1. Build CeleryToGo:

```bash
make build
```

2. Start CeleryToGo with the custom pool configuration:

```bash
./build/celeryToGo --config=examples/custom-pool/config.json
```

## Process Pool Monitoring

CeleryToGo provides metrics for monitoring the process pool:

- Total number of processes
- Number of idle processes
- Number of busy processes
- Process age and task count
- Memory usage per process

You can view these metrics at the configured metrics endpoint (default: http://localhost:9090/metrics).

## Explanation

The process pool configuration should be tuned based on:

1. **Task Characteristics**: Memory usage, CPU usage, and duration.
2. **Hardware Resources**: Available CPU cores and memory.
3. **Throughput Requirements**: Number of tasks per second.

### Process Recycling Strategy

Processes are recycled when:

1. They've processed `max_tasks_per_process` tasks.
2. They've been idle for `max_idle_time`.
3. They've been alive for `max_process_age`.
4. They encounter an error.

This ensures that resources are used efficiently and prevents memory leaks or resource exhaustion.

## Best Practices

1. **Start with Conservative Settings**: Begin with lower values and increase as needed.
2. **Monitor Resource Usage**: Watch CPU, memory, and process counts.
3. **Adjust Based on Task Characteristics**: Different task types may need different configurations.
4. **Consider Process Startup Time**: If tasks are short but process startup is expensive, keep more idle processes.
5. **Balance Throughput and Resource Usage**: More processes increase throughput but also resource consumption.