# MongoDB Sample Data Generator

This Go program (`data_generator.go`) is designed to populate a MongoDB database with sample time-series data suitable for testing the `promql2mongo` bridge.

## Functionality

1.  **Connects to MongoDB:** It takes a MongoDB connection URL as a command-line argument (`-url`, defaults to `mongodb://localhost:27017`) and establishes a connection.
2.  **Clears Existing Data:** Before generating new data, it removes all existing documents from the target collections (`metrics_http`, `metrics_system`, `metrics_memory`) within the `metrics_db` database.
3.  **Generates HTTP Metrics (`metrics_http` collection):**
    *   Creates documents simulating HTTP request metrics.
    *   Includes fields like `timestamp`, `metric_name` (alternating between `http_requests_total` and `http_request_duration_seconds`), `value` (random float), `status_code`, `http_method`, `endpoint`, and `server_id`.
    *   Generates a small number of records with timestamps decreasing by minutes from the current time.
4.  **Generates CPU Metrics (`metrics_system` collection):**
    *   Creates documents simulating node CPU usage (`node_cpu_seconds_total`).
    *   Includes fields like `ts` (timestamp), `name` (metric name), `value` (random float, weighted by mode), `cpu_mode` (user, system, idle, etc.), `cpu_id`, and `host`.
    *   Simulates multiple CPUs per host and various CPU states.
    *   Generates records with timestamps decreasing by minutes.
5.  **Generates Memory Metrics (`metrics_memory` collection):**
    *   Creates documents simulating node memory usage (`node_memory_usage_bytes`).
    *   Includes fields like `time` (timestamp), `metric` (metric name), `value` (calculated based on type and simulated total/used), `memory_type` (used, free, cached, etc.), and `host_id`.
    *   Simulates multiple hosts with a basic memory usage pattern over time.
    *   Generates records with timestamps decreasing by hours.

## Purpose

The primary goal is to create realistic-looking sample data in MongoDB that mirrors the structure defined in the `config.yaml` for the `promql2mongo` bridge, allowing users to test queries against the bridge without needing a real data source.