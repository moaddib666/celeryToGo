// Package main provides the entry point for the Celery Go worker.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"celeryToGo/internal/broker"
	sqsbroker "celeryToGo/internal/broker/sqs"
	"celeryToGo/internal/observability/logging"
	"celeryToGo/internal/observability/metrics"
	"celeryToGo/internal/observability/tracing"
	"celeryToGo/internal/pool"
	"celeryToGo/internal/pool/python"
	"celeryToGo/internal/protocol/celery"
	"celeryToGo/internal/worker"
)

// Config contains the configuration for the worker.
type Config struct {
	// Broker configuration
	QueueURL          string
	Region            string
	Endpoint          string
	MaxMessages       int
	VisibilityTimeout int
	WaitTimeSeconds   int
	MessageGroupID    string
	DeduplicationID   string

	// Worker configuration
	Concurrency       int
	QueuePrefetch     int
	TaskTimeout       time.Duration
	ShutdownTimeout   time.Duration
	HeartbeatInterval time.Duration

	// Process pool configuration
	MinProcesses           int
	MaxProcesses           int
	MaxTasksPerProcess     int64
	MaxIdleTime            time.Duration
	MaxProcessAge          time.Duration
	ProcessStartupTimeout  time.Duration
	ProcessShutdownTimeout time.Duration
	PythonPath             string
	WorkerScriptPath       string
	ProcessEnv             []string

	// Observability configuration
	LogLevel        string
	MetricsEnabled  bool
	MetricsEndpoint string
	TracingEnabled  bool
	TracingEndpoint string
	ServiceName     string
	ServiceVersion  string
	Environment     string
}

// parseFlags parses the command line flags.
func parseFlags() *Config {
	config := &Config{}

	// Broker configuration
	flag.StringVar(&config.QueueURL, "queue-url", "", "URL of the SQS queue")
	flag.StringVar(&config.Region, "region", "us-east-1", "AWS region")
	flag.StringVar(&config.Endpoint, "endpoint", "", "SQS endpoint URL (for local development)")
	flag.IntVar(&config.MaxMessages, "max-messages", 10, "Maximum number of messages to receive in a single call")
	flag.IntVar(&config.VisibilityTimeout, "visibility-timeout", 30, "Visibility timeout for messages in seconds")
	flag.IntVar(&config.WaitTimeSeconds, "wait-time", 20, "Wait time for long polling in seconds")
	flag.StringVar(&config.MessageGroupID, "message-group-id", "", "Message group ID for FIFO queues")
	flag.StringVar(&config.DeduplicationID, "deduplication-id", "", "Deduplication ID for FIFO queues")

	// Worker configuration
	flag.IntVar(&config.Concurrency, "concurrency", 10, "Maximum number of tasks to process concurrently")
	flag.IntVar(&config.QueuePrefetch, "queue-prefetch", 10, "Number of messages to prefetch from the queue")
	flag.DurationVar(&config.TaskTimeout, "task-timeout", 5*time.Minute, "Default timeout for task execution")
	flag.DurationVar(&config.ShutdownTimeout, "shutdown-timeout", 30*time.Second, "Maximum time to wait for tasks to complete during shutdown")
	flag.DurationVar(&config.HeartbeatInterval, "heartbeat-interval", 10*time.Second, "Interval at which to send heartbeats")

	// Process pool configuration
	flag.IntVar(&config.MinProcesses, "min-processes", 1, "Minimum number of processes to keep in the pool")
	flag.IntVar(&config.MaxProcesses, "max-processes", 10, "Maximum number of processes to allow in the pool")
	flag.Int64Var(&config.MaxTasksPerProcess, "max-tasks-per-process", 100, "Maximum number of tasks a process can execute before being recycled")
	flag.DurationVar(&config.MaxIdleTime, "max-idle-time", 5*time.Minute, "Maximum time a process can be idle before being recycled")
	flag.DurationVar(&config.MaxProcessAge, "max-process-age", 1*time.Hour, "Maximum age of a process before being recycled")
	flag.DurationVar(&config.ProcessStartupTimeout, "process-startup-timeout", 30*time.Second, "Maximum time to wait for a process to start")
	flag.DurationVar(&config.ProcessShutdownTimeout, "process-shutdown-timeout", 30*time.Second, "Maximum time to wait for a process to shut down gracefully")
	flag.StringVar(&config.PythonPath, "python-path", "python", "Path to the Python executable")
	flag.StringVar(&config.WorkerScriptPath, "worker-script-path", "scripts/python/worker_process.py", "Path to the Python worker script")

	// Observability configuration
	flag.StringVar(&config.LogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.BoolVar(&config.MetricsEnabled, "metrics-enabled", true, "Enable Prometheus metrics")
	flag.StringVar(&config.MetricsEndpoint, "metrics-endpoint", ":9090", "Endpoint for Prometheus metrics")
	flag.BoolVar(&config.TracingEnabled, "tracing-enabled", false, "Enable OpenTelemetry tracing")
	flag.StringVar(&config.TracingEndpoint, "tracing-endpoint", "localhost:4317", "Endpoint for OpenTelemetry tracing")
	flag.StringVar(&config.ServiceName, "service-name", "celery-go-worker", "Service name for tracing")
	flag.StringVar(&config.ServiceVersion, "service-version", "0.1.0", "Service version for tracing")
	flag.StringVar(&config.Environment, "environment", "development", "Environment (development, staging, production)")

	flag.Parse()

	// Validate required flags
	if config.QueueURL == "" {
		fmt.Println("Error: queue-url is required")
		flag.Usage()
		os.Exit(1)
	}

	// Resolve worker script path
	if !filepath.IsAbs(config.WorkerScriptPath) {
		absPath, err := filepath.Abs(config.WorkerScriptPath)
		if err == nil {
			config.WorkerScriptPath = absPath
		}
	}

	// Add environment variables from the current process
	config.ProcessEnv = os.Environ()

	return config
}

// setupObservability sets up the observability components.
func setupObservability(config *Config) {
	// Set up logging
	logging.SetLevel(config.LogLevel)
	logging.Info("Logging initialized with level:", config.LogLevel)

	// Set up metrics
	metrics.InitMetrics(metrics.Config{
		Enabled:      config.MetricsEnabled,
		Namespace:    "celery_go",
		HTTPEndpoint: config.MetricsEndpoint,
	})
	logging.Info("Metrics initialized")

	// Set up tracing
	if config.TracingEnabled {
		err := tracing.InitTracer(tracing.Config{
			Enabled:        true,
			ServiceName:    config.ServiceName,
			ServiceVersion: config.ServiceVersion,
			Environment:    config.Environment,
			OTLPEndpoint:   config.TracingEndpoint,
			SampleRate:     1.0,
		})
		if err != nil {
			logging.WithError(err).Error("Failed to initialize tracer")
		} else {
			logging.Info("Tracing initialized")
		}
	}
}

// setupBroker sets up the message broker.
func setupBroker(config *Config) (broker.Broker, error) {
	// Create AWS configuration
	awsConfig, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(config.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create SQS client
	var sqsClient *sqs.Client
	if config.Endpoint != "" {
		// Use custom endpoint for local development
		sqsClient = sqs.NewFromConfig(awsConfig, func(o *sqs.Options) {
			o.BaseEndpoint = &config.Endpoint
		})
	} else {
		sqsClient = sqs.NewFromConfig(awsConfig)
	}

	// Create broker configuration
	brokerConfig := broker.Config{
		QueueURL:          config.QueueURL,
		Region:            config.Region,
		Endpoint:          config.Endpoint,
		MaxMessages:       config.MaxMessages,
		VisibilityTimeout: config.VisibilityTimeout,
		WaitTimeSeconds:   config.WaitTimeSeconds,
	}

	// Create SQS FIFO broker
	return sqsbroker.NewSQSFIFOBroker(sqsClient, brokerConfig)
}

// setupProcessPool sets up the process pool.
func setupProcessPool(config *Config) (pool.Pool, error) {
	// Create protocol
	protocol := celery.NewJSONProtocol()

	// Create pool configuration
	poolConfig := pool.PoolConfig{
		MinProcesses:           config.MinProcesses,
		MaxProcesses:           config.MaxProcesses,
		MaxTasksPerProcess:     config.MaxTasksPerProcess,
		MaxIdleTime:            config.MaxIdleTime,
		MaxProcessAge:          config.MaxProcessAge,
		ProcessStartupTimeout:  config.ProcessStartupTimeout,
		ProcessShutdownTimeout: config.ProcessShutdownTimeout,
		TaskExecutionTimeout:   config.TaskTimeout,
		PythonPath:             config.PythonPath,
		WorkerScriptPath:       config.WorkerScriptPath,
		ProcessEnv:             config.ProcessEnv,
	}

	// Create process pool
	return python.NewProcessPool(poolConfig, protocol)
}

// setupWorker sets up the worker.
func setupWorker(config *Config, broker broker.Broker, pool pool.Pool) (worker.Worker, error) {
	// Create protocol
	protocol := celery.NewJSONProtocol()

	// Create worker configuration
	workerConfig := worker.WorkerConfig{
		Concurrency:       config.Concurrency,
		QueuePrefetch:     config.QueuePrefetch,
		TaskTimeout:       config.TaskTimeout,
		ShutdownTimeout:   config.ShutdownTimeout,
		HeartbeatInterval: config.HeartbeatInterval,
		RetryBackoff: worker.RetryBackoff{
			InitialInterval:     1 * time.Second,
			MaxInterval:         30 * time.Second,
			Multiplier:          2.0,
			RandomizationFactor: 0.5,
		},
	}

	// Create worker factory
	factory := worker.NewDefaultWorkerFactory()

	// Create worker
	return factory.CreateWorker(context.Background(), broker, pool, protocol, workerConfig)
}

func main() {
	// Parse command line flags
	config := parseFlags()

	// Set up observability
	setupObservability(config)

	// Create context for the application
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-signalChan
		logging.WithField("signal", sig).Info("Received termination signal")
		cancel()
	}()

	// Set up broker
	broker, err := setupBroker(config)
	if err != nil {
		logging.WithError(err).Fatal("Failed to set up broker")
	}

	// Connect to broker
	if err := broker.Connect(ctx); err != nil {
		logging.WithError(err).Fatal("Failed to connect to broker")
	}
	defer broker.Close(ctx)

	// Set up process pool
	pool, err := setupProcessPool(config)
	if err != nil {
		logging.WithError(err).Fatal("Failed to set up process pool")
	}

	// Start process pool
	if err := pool.Start(ctx); err != nil {
		logging.WithError(err).Fatal("Failed to start process pool")
	}
	defer pool.Stop(ctx)

	// Set up worker
	worker, err := setupWorker(config, broker, pool)
	if err != nil {
		logging.WithError(err).Fatal("Failed to set up worker")
	}

	// Start worker
	if err := worker.Start(ctx); err != nil {
		logging.WithError(err).Fatal("Failed to start worker")
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Create context with timeout for shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer shutdownCancel()

	// Stop worker
	logging.Info("Stopping worker")
	if err := worker.Stop(shutdownCtx); err != nil {
		logging.WithError(err).Error("Failed to stop worker")
	}

	// Stop process pool
	logging.Info("Stopping process pool")
	if err := pool.Stop(shutdownCtx); err != nil {
		logging.WithError(err).Error("Failed to stop process pool")
	}

	// Close broker connection
	logging.Info("Closing broker connection")
	if err := broker.Close(shutdownCtx); err != nil {
		logging.WithError(err).Error("Failed to close broker connection")
	}

	// Shutdown observability
	logging.Info("Shutting down observability")
	if err := metrics.Shutdown(); err != nil {
		logging.WithError(err).Error("Failed to shut down metrics")
	}
	if err := tracing.Shutdown(shutdownCtx); err != nil {
		logging.WithError(err).Error("Failed to shut down tracing")
	}

	logging.Info("Worker shutdown complete")
}
