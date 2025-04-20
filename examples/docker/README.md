# Docker-Based Deployment Example

This example demonstrates how to deploy CeleryToGo using Docker and Docker Compose, including integration with AWS SQS and observability tools.

## Prerequisites

- Docker and Docker Compose
- AWS credentials
- An SQS FIFO queue

## Files in this Example

- `Dockerfile`: Builds the CeleryToGo container
- `docker-compose.yml`: Orchestrates CeleryToGo with supporting services
- `config.json`: Configuration for CeleryToGo
- `.env.example`: Example environment variables (copy to `.env` and customize)

## Dockerfile

The Dockerfile builds a multi-stage container that:

1. Builds the Go binary in a builder stage
2. Installs Python and required dependencies
3. Creates a minimal runtime image with both Go and Python

## Docker Compose Setup

The docker-compose.yml file sets up:

1. CeleryToGo worker
2. Prometheus for metrics collection
3. Jaeger for distributed tracing
4. ElasticMQ for local SQS testing (optional)

## Configuration

The configuration is provided through:

1. Environment variables (passed to the container)
2. A mounted config.json file
3. Docker Compose service configuration

## Running the Example

1. Copy the example environment file:

```bash
cp examples/docker/.env.example examples/docker/.env
```

2. Edit the `.env` file with your AWS credentials and configuration.

3. Start the services:

```bash
cd examples/docker
docker-compose up -d
```

4. View the logs:

```bash
docker-compose logs -f celery-to-go
```

5. Access the observability tools:

- Prometheus: http://localhost:9090
- Jaeger: http://localhost:16686

6. Submit tasks to your SQS queue and watch them being processed.

7. Shut down the services:

```bash
docker-compose down
```

## Using with Local SQS (ElasticMQ)

For development and testing, you can use ElasticMQ as a local SQS replacement:

1. Uncomment the ElasticMQ service in docker-compose.yml
2. Update the queue URL in config.json to point to ElasticMQ
3. Start the services as described above

## Production Deployment Considerations

For production deployments:

1. **Security**: Use a non-root user in the container
2. **Resource Limits**: Set appropriate CPU and memory limits
3. **Persistence**: Configure volume mounts for any persistent data
4. **Networking**: Configure appropriate network security
5. **Secrets Management**: Use Docker secrets or a vault service for credentials
6. **Health Checks**: Implement and configure container health checks
7. **Logging**: Configure log drivers for centralized logging

## Kubernetes Deployment

For Kubernetes deployment:

1. Build and push the Docker image to a container registry
2. Create Kubernetes manifests (Deployment, Service, ConfigMap, Secret)
3. Apply the manifests to your Kubernetes cluster

Example Kubernetes manifests are provided in the `kubernetes/` directory.

## Explanation

This Docker-based example demonstrates:

1. How to containerize CeleryToGo with all dependencies
2. How to configure the container for different environments
3. How to integrate with observability tools
4. How to set up local development with ElasticMQ

This approach provides a portable, reproducible deployment that can be used in various environments from development to production.