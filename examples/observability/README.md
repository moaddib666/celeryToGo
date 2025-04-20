# Observability Setup Example

This example demonstrates how to configure CeleryToGo for comprehensive observability using structured logging, Prometheus metrics, and OpenTelemetry tracing.

## Prerequisites

- Go 1.23 or higher
- Python 3.6 or higher
- AWS credentials configured
- An SQS queue
- Prometheus server (optional, for metrics collection)
- Jaeger or other OpenTelemetry collector (optional, for distributed tracing)

## Observability Components

CeleryToGo provides three main observability components:

1. **Structured Logging**: JSON-formatted logs with Logrus
2. **Metrics**: Prometheus metrics for monitoring
3. **Distributed Tracing**: OpenTelemetry tracing for request flows

## Configuration

The observability components can be configured with the following options:

```bash
./celeryToGo \
  --queue-url=https://sqs.us-east-1.amazonaws.com/123456789012/my-queue.fifo \
  --log-level=debug \
  --metrics-enabled=true \
  --metrics-endpoint=:9090 \
  --tracing-enabled=true \
  --tracing-endpoint=localhost:4317 \
  --service-name=celery-go-worker \
  --service-version=0.1.0 \
  --environment=production
```

## Structured Logging

CeleryToGo uses Logrus for structured logging with JSON output. This makes logs easier to parse and analyze with tools like ELK Stack or CloudWatch Logs.

Example log output:

```json
{
  "level": "info",
  "msg": "Task executed successfully",
  "task_id": "12345678-1234-1234-1234-123456789012",
  "task_name": "app.tasks.process_order",
  "runtime_seconds": 0.235,
  "timestamp": "2023-05-15T12:34:56Z"
}
```

### Log Levels

- `debug`: Detailed information for debugging
- `info`: General operational information
- `warn`: Warning events that might cause issues
- `error`: Error events that might still allow the application to continue
- `fatal`: Severe error events that cause the application to terminate

## Prometheus Metrics

CeleryToGo exposes Prometheus metrics at the configured endpoint (default: `:9090`). These metrics can be scraped by a Prometheus server and visualized with tools like Grafana.

### Available Metrics

- **Task Metrics**:
  - `celery_go_tasks_received_total`: Total number of tasks received
  - `celery_go_tasks_processed_total`: Total number of tasks processed
  - `celery_go_tasks_succeeded_total`: Total number of tasks that succeeded
  - `celery_go_tasks_failed_total`: Total number of tasks that failed
  - `celery_go_tasks_retried_total`: Total number of tasks that were retried
  - `celery_go_task_duration_seconds`: The duration of task execution in seconds

- **Process Metrics**:
  - `celery_go_processes_total`: Total number of processes
  - `celery_go_processes_idle`: Number of idle processes
  - `celery_go_processes_busy`: Number of busy processes
  - `celery_go_processes_starting`: Number of processes that are starting up
  - `celery_go_processes_stopping`: Number of processes that are shutting down
  - `celery_go_processes_error`: Number of processes in an error state

- **Message Group Metrics**:
  - `celery_go_message_groups_active`: Number of active message groups
  - `celery_go_message_groups_processing`: Number of message groups being processed

### Prometheus Configuration

Example Prometheus configuration to scrape CeleryToGo metrics:

```yaml
scrape_configs:
  - job_name: 'celery-go-worker'
    scrape_interval: 15s
    static_configs:
      - targets: ['localhost:9090']
```

## OpenTelemetry Tracing

CeleryToGo supports distributed tracing with OpenTelemetry. This allows you to trace the execution of tasks across different services.

### Trace Spans

CeleryToGo creates spans for:

- Receiving messages from the broker
- Processing tasks
- Executing tasks in Python processes
- Acknowledging messages

### Jaeger Configuration

Example Jaeger configuration to collect traces from CeleryToGo:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317

processors:
  batch:
    timeout: 1s
    send_batch_size: 1024

exporters:
  jaeger:
    endpoint: jaeger:14250
    tls:
      insecure: true

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [jaeger]
```

## Running the Example

1. Start Prometheus (optional):

```bash
docker run -d -p 9090:9090 -v $(pwd)/examples/observability/prometheus.yml:/etc/prometheus/prometheus.yml prom/prometheus
```

2. Start Jaeger (optional):

```bash
docker run -d -p 16686:16686 -p 4317:4317 -p 4318:4318 jaegertracing/all-in-one:latest
```

3. Build CeleryToGo:

```bash
make build
```

4. Start CeleryToGo with observability configuration:

```bash
./build/celeryToGo --config=examples/observability/config.json
```

5. View the logs:

```bash
# Logs are output to stdout by default
```

6. View metrics in Prometheus UI:

```
http://localhost:9090
```

7. View traces in Jaeger UI:

```
http://localhost:16686
```

## Explanation

In this observability example:

1. CeleryToGo outputs structured logs in JSON format
2. Prometheus metrics are exposed for monitoring
3. OpenTelemetry traces are sent to a collector

This configuration is suitable for production environments where comprehensive monitoring and debugging capabilities are required.

## Best Practices

1. **Use Structured Logging**: Always use structured logging in production for easier parsing and analysis.
2. **Set Appropriate Log Levels**: Use `info` in production and `debug` for troubleshooting.
3. **Monitor Key Metrics**: Set up alerts for critical metrics like task failure rate and process errors.
4. **Use Distributed Tracing**: Enable tracing to debug complex issues across services.
5. **Visualize Metrics**: Use Grafana or similar tools to create dashboards for monitoring.