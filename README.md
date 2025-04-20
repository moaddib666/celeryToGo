# CeleryToGo
CeleryToGo is a Go-based Celery worker replacement that executes Python Celery tasks with a focus on reliability, performance, and AWS SQS FIFO queue support.

## DISCLAIMER

**This project is not affiliated with or endorsed by the Celery project.**

The build is experimental and **not intended for production use**. Please use at your own risk.

If you want to participate in the development of this project, please reach out to me on GitHub or create an issue.

Stars would show your interest in this project and help me to keep it alive.

## Usage

This guide will help you get started with the project:

1. **Build the binary:**
   ```bash
   make build
   ```

2. **Copy the binary to your Python project root**

3. **Add the Python entrypoint script to your project:**
   ```bash
   cp scripts/python/worker_process.py <your_python_project_root>/scripts/python/worker_process.py
   ```

4. **Set up LocalStack for local development:**
   ```bash
   docker run -d -p 4566:4566 -p 4510-4559:4510-4559 localstack/localstack
   ```

5. **Create a SQS FIFO queue for Celery** (or use an existing one)

6. **Run the worker:**
   ```bash
   ./celeryToGo-amd64-darwin -queue-url http://sqs.us-west-2.localhost.localstack.cloud:4566/000000000000/{QUEUE_NAME}.fifo -log-level debug
   ```

## Features

- **SQS FIFO Support**
  - Proper message group handling to maintain ordering
  - Support for batched operations while preserving FIFO semantics

- **Process Management**
  - Pool of Python processes to execute tasks
  - Environment variable propagation
  - Unix signal forwarding
  - Process health monitoring and recycling

- **IPC Protocol**
  - Structured JSON-based protocol for Go-Python communication
  - Support for task execution, result retrieval, and process control
  - Timeout handling and error recovery

- **Observability**
  - Structured logging with Logrus
  - Distributed tracing with OpenTelemetry
  - Prometheus metrics for monitoring

## Architecture

CeleryToGo follows a clean architecture with well-defined interfaces:

```
┌───────────────────┐     ┌──────────────────┐     ┌───────────────────┐
│                   │     │                  │     │                   │
│  Message Broker   │◄────┤  Worker Manager  │────►│  Process Pool     │
│  (SQS FIFO)       │     │                  │     │                   │
│                   │     │                  │     │                   │
└───────────────────┘     └──────────────────┘     └───────────────────┘
                                   ▲                        │
                                   │                        │
                                   │                        ▼
                          ┌──────────────────┐     ┌───────────────────┐
                          │                  │     │                   │
                          │  Observability   │     │  Python Processes │
                          │  (Logs, Metrics, │     │  (Task Executors) │
                          │   Traces)        │     │                   │
                          └──────────────────┘     └───────────────────┘
```

### Components

1. **Message Broker**: Handles communication with SQS FIFO queues, maintaining message ordering.
2. **Worker Manager**: Coordinates task processing, manages retries, and handles graceful shutdown.
3. **Process Pool**: Manages Python processes, monitors health, and recycles processes as needed.
4. **Python Processes**: Execute Celery tasks in a Python environment.
5. **Observability**: Provides logging, metrics, and tracing for monitoring and debugging.

## Development

### Building

```bash
# Build the binary
make build

# Build for specific platforms
make build-linux
make build-darwin
```

### Testing

```bash
# Run all tests
make test

# Run unit tests
make test-unit

# Run integration tests
make test-integration
```

### Linting

```bash
# Run linters
make lint
```

## Releases

CeleryToGo uses GitHub Actions to automatically build and publish binaries when a new tag is pushed to the repository. The following platforms are supported:

- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64)

To download pre-built binaries, visit the [Releases](https://github.com/moaddib666/celeryToGo/releases) page.

### Creating a Release

To create a new release:

1. Tag the commit with a version number:
   ```bash
   git tag v1.0.0
   ```

2. Push the tag to GitHub:
   ```bash
   git push origin v1.0.0
   ```

3. GitHub Actions will automatically build the binaries and create a release with the artifacts.

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
