# Stage 1: Build the Go binary
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o celeryToGo ./cmd/worker

# Stage 2: Create the runtime image
FROM python:3.9-slim

WORKDIR /app

# Install required packages
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Copy the Go binary from the builder stage
COPY --from=builder /app/celeryToGo /app/celeryToGo

# Copy the Python worker script
COPY scripts/python/worker_process.py /app/scripts/python/worker_process.py

# Create a non-root user
RUN useradd -m celeryuser
RUN chown -R celeryuser:celeryuser /app
USER celeryuser

# Set environment variables
ENV PYTHONUNBUFFERED=1

# Expose the metrics port
EXPOSE 9090

# Set the entrypoint
ENTRYPOINT ["/app/celeryToGo"]

# Default command (can be overridden)
CMD ["--help"]