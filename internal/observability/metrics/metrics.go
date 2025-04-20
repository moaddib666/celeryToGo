// Package metrics provides metrics utilities for the application.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"celeryToGo/internal/observability/logging"
)

// Config contains configuration for the metrics.
type Config struct {
	// Enabled indicates whether metrics are enabled.
	Enabled bool

	// Namespace is the namespace for the metrics.
	Namespace string

	// Subsystem is the subsystem for the metrics.
	Subsystem string

	// HTTPEndpoint is the endpoint for the HTTP server.
	HTTPEndpoint string
}

// Metrics contains the metrics for the application.
type Metrics struct {
	// TasksReceived is the number of tasks received.
	TasksReceived *prometheus.CounterVec

	// TasksProcessed is the number of tasks processed.
	TasksProcessed *prometheus.CounterVec

	// TasksSucceeded is the number of tasks that succeeded.
	TasksSucceeded *prometheus.CounterVec

	// TasksFailed is the number of tasks that failed.
	TasksFailed *prometheus.CounterVec

	// TasksRetried is the number of tasks that were retried.
	TasksRetried *prometheus.CounterVec

	// TaskDuration is the duration of task execution.
	TaskDuration *prometheus.HistogramVec

	// ProcessesTotal is the total number of processes.
	ProcessesTotal prometheus.Gauge

	// ProcessesIdle is the number of idle processes.
	ProcessesIdle prometheus.Gauge

	// ProcessesBusy is the number of busy processes.
	ProcessesBusy prometheus.Gauge

	// ProcessesStarting is the number of processes that are starting up.
	ProcessesStarting prometheus.Gauge

	// ProcessesStopping is the number of processes that are shutting down.
	ProcessesStopping prometheus.Gauge

	// ProcessesError is the number of processes in an error state.
	ProcessesError prometheus.Gauge

	// MessageGroupsActive is the number of active message groups.
	MessageGroupsActive prometheus.Gauge

	// MessageGroupsProcessing is the number of message groups being processed.
	MessageGroupsProcessing prometheus.Gauge

	// config is the configuration for the metrics.
	config Config

	// server is the HTTP server for the metrics.
	server *http.Server
}

// NewMetrics creates a new Metrics.
func NewMetrics(config Config) *Metrics {
	if !config.Enabled {
		return &Metrics{
			config: config,
		}
	}

	// Set default values
	if config.Namespace == "" {
		config.Namespace = "celery_go"
	}
	if config.HTTPEndpoint == "" {
		config.HTTPEndpoint = ":9090"
	}

	// Create the metrics
	metrics := &Metrics{
		TasksReceived: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "tasks_received_total",
			Help:      "The total number of tasks received",
		}, []string{"queue", "task_name"}),

		TasksProcessed: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "tasks_processed_total",
			Help:      "The total number of tasks processed",
		}, []string{"queue", "task_name"}),

		TasksSucceeded: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "tasks_succeeded_total",
			Help:      "The total number of tasks that succeeded",
		}, []string{"queue", "task_name"}),

		TasksFailed: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "tasks_failed_total",
			Help:      "The total number of tasks that failed",
		}, []string{"queue", "task_name"}),

		TasksRetried: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "tasks_retried_total",
			Help:      "The total number of tasks that were retried",
		}, []string{"queue", "task_name"}),

		TaskDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "task_duration_seconds",
			Help:      "The duration of task execution in seconds",
			Buckets:   prometheus.DefBuckets,
		}, []string{"queue", "task_name"}),

		ProcessesTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "processes_total",
			Help:      "The total number of processes",
		}),

		ProcessesIdle: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "processes_idle",
			Help:      "The number of idle processes",
		}),

		ProcessesBusy: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "processes_busy",
			Help:      "The number of busy processes",
		}),

		ProcessesStarting: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "processes_starting",
			Help:      "The number of processes that are starting up",
		}),

		ProcessesStopping: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "processes_stopping",
			Help:      "The number of processes that are shutting down",
		}),

		ProcessesError: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "processes_error",
			Help:      "The number of processes in an error state",
		}),

		MessageGroupsActive: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "message_groups_active",
			Help:      "The number of active message groups",
		}),

		MessageGroupsProcessing: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "message_groups_processing",
			Help:      "The number of message groups being processed",
		}),

		config: config,
	}

	// Start the HTTP server
	if config.HTTPEndpoint != "" {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())

		server := &http.Server{
			Addr:    config.HTTPEndpoint,
			Handler: mux,
		}

		go func() {
			logging.Infof("Starting metrics HTTP server on %s", config.HTTPEndpoint)
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logging.Errorf("Metrics HTTP server error: %v", err)
			}
		}()

		metrics.server = server
	}

	logging.Info("Prometheus metrics initialized")

	return metrics
}

// ObserveTaskReceived observes a task being received.
func (m *Metrics) ObserveTaskReceived(queue, taskName string) {
	if !m.config.Enabled {
		return
	}
	m.TasksReceived.WithLabelValues(queue, taskName).Inc()
}

// ObserveTaskProcessed observes a task being processed.
func (m *Metrics) ObserveTaskProcessed(queue, taskName string) {
	if !m.config.Enabled {
		return
	}
	m.TasksProcessed.WithLabelValues(queue, taskName).Inc()
}

// ObserveTaskSucceeded observes a task succeeding.
func (m *Metrics) ObserveTaskSucceeded(queue, taskName string) {
	if !m.config.Enabled {
		return
	}
	m.TasksSucceeded.WithLabelValues(queue, taskName).Inc()
}

// ObserveTaskFailed observes a task failing.
func (m *Metrics) ObserveTaskFailed(queue, taskName string) {
	if !m.config.Enabled {
		return
	}
	m.TasksFailed.WithLabelValues(queue, taskName).Inc()
}

// ObserveTaskRetried observes a task being retried.
func (m *Metrics) ObserveTaskRetried(queue, taskName string) {
	if !m.config.Enabled {
		return
	}
	m.TasksRetried.WithLabelValues(queue, taskName).Inc()
}

// ObserveTaskDuration observes the duration of a task.
func (m *Metrics) ObserveTaskDuration(queue, taskName string, duration time.Duration) {
	if !m.config.Enabled {
		return
	}
	m.TaskDuration.WithLabelValues(queue, taskName).Observe(duration.Seconds())
}

// SetProcessesTotal sets the total number of processes.
func (m *Metrics) SetProcessesTotal(count int) {
	if !m.config.Enabled {
		return
	}
	m.ProcessesTotal.Set(float64(count))
}

// SetProcessesIdle sets the number of idle processes.
func (m *Metrics) SetProcessesIdle(count int) {
	if !m.config.Enabled {
		return
	}
	m.ProcessesIdle.Set(float64(count))
}

// SetProcessesBusy sets the number of busy processes.
func (m *Metrics) SetProcessesBusy(count int) {
	if !m.config.Enabled {
		return
	}
	m.ProcessesBusy.Set(float64(count))
}

// SetProcessesStarting sets the number of processes that are starting up.
func (m *Metrics) SetProcessesStarting(count int) {
	if !m.config.Enabled {
		return
	}
	m.ProcessesStarting.Set(float64(count))
}

// SetProcessesStopping sets the number of processes that are shutting down.
func (m *Metrics) SetProcessesStopping(count int) {
	if !m.config.Enabled {
		return
	}
	m.ProcessesStopping.Set(float64(count))
}

// SetProcessesError sets the number of processes in an error state.
func (m *Metrics) SetProcessesError(count int) {
	if !m.config.Enabled {
		return
	}
	m.ProcessesError.Set(float64(count))
}

// SetMessageGroupsActive sets the number of active message groups.
func (m *Metrics) SetMessageGroupsActive(count int) {
	if !m.config.Enabled {
		return
	}
	m.MessageGroupsActive.Set(float64(count))
}

// SetMessageGroupsProcessing sets the number of message groups being processed.
func (m *Metrics) SetMessageGroupsProcessing(count int) {
	if !m.config.Enabled {
		return
	}
	m.MessageGroupsProcessing.Set(float64(count))
}

// Shutdown shuts down the metrics.
func (m *Metrics) Shutdown() error {
	if m.server != nil {
		logging.Info("Shutting down metrics HTTP server")
		return m.server.Close()
	}
	return nil
}

// DefaultMetrics is the default metrics for the application.
var DefaultMetrics *Metrics

// InitMetrics initializes the default metrics.
func InitMetrics(config Config) {
	DefaultMetrics = NewMetrics(config)
}

// ObserveTaskReceived observes a task being received using the default metrics.
func ObserveTaskReceived(queue, taskName string) {
	if DefaultMetrics != nil {
		DefaultMetrics.ObserveTaskReceived(queue, taskName)
	}
}

// ObserveTaskProcessed observes a task being processed using the default metrics.
func ObserveTaskProcessed(queue, taskName string) {
	if DefaultMetrics != nil {
		DefaultMetrics.ObserveTaskProcessed(queue, taskName)
	}
}

// ObserveTaskSucceeded observes a task succeeding using the default metrics.
func ObserveTaskSucceeded(queue, taskName string) {
	if DefaultMetrics != nil {
		DefaultMetrics.ObserveTaskSucceeded(queue, taskName)
	}
}

// ObserveTaskFailed observes a task failing using the default metrics.
func ObserveTaskFailed(queue, taskName string) {
	if DefaultMetrics != nil {
		DefaultMetrics.ObserveTaskFailed(queue, taskName)
	}
}

// ObserveTaskRetried observes a task being retried using the default metrics.
func ObserveTaskRetried(queue, taskName string) {
	if DefaultMetrics != nil {
		DefaultMetrics.ObserveTaskRetried(queue, taskName)
	}
}

// ObserveTaskDuration observes the duration of a task using the default metrics.
func ObserveTaskDuration(queue, taskName string, duration time.Duration) {
	if DefaultMetrics != nil {
		DefaultMetrics.ObserveTaskDuration(queue, taskName, duration)
	}
}

// SetProcessesTotal sets the total number of processes using the default metrics.
func SetProcessesTotal(count int) {
	if DefaultMetrics != nil {
		DefaultMetrics.SetProcessesTotal(count)
	}
}

// SetProcessesIdle sets the number of idle processes using the default metrics.
func SetProcessesIdle(count int) {
	if DefaultMetrics != nil {
		DefaultMetrics.SetProcessesIdle(count)
	}
}

// SetProcessesBusy sets the number of busy processes using the default metrics.
func SetProcessesBusy(count int) {
	if DefaultMetrics != nil {
		DefaultMetrics.SetProcessesBusy(count)
	}
}

// SetProcessesStarting sets the number of processes that are starting up using the default metrics.
func SetProcessesStarting(count int) {
	if DefaultMetrics != nil {
		DefaultMetrics.SetProcessesStarting(count)
	}
}

// SetProcessesStopping sets the number of processes that are shutting down using the default metrics.
func SetProcessesStopping(count int) {
	if DefaultMetrics != nil {
		DefaultMetrics.SetProcessesStopping(count)
	}
}

// SetProcessesError sets the number of processes in an error state using the default metrics.
func SetProcessesError(count int) {
	if DefaultMetrics != nil {
		DefaultMetrics.SetProcessesError(count)
	}
}

// SetMessageGroupsActive sets the number of active message groups using the default metrics.
func SetMessageGroupsActive(count int) {
	if DefaultMetrics != nil {
		DefaultMetrics.SetMessageGroupsActive(count)
	}
}

// SetMessageGroupsProcessing sets the number of message groups being processed using the default metrics.
func SetMessageGroupsProcessing(count int) {
	if DefaultMetrics != nil {
		DefaultMetrics.SetMessageGroupsProcessing(count)
	}
}

// Shutdown shuts down the default metrics.
func Shutdown() error {
	if DefaultMetrics != nil {
		return DefaultMetrics.Shutdown()
	}
	return nil
}