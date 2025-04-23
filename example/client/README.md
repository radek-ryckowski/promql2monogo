# Prometheus API Query Client

This command-line tool (`client.go`) allows you to send PromQL queries (both instant and range) to any Prometheus-compatible HTTP API endpoint, such as the `promql2mongo` bridge or a standard Prometheus server.

## Features

*   Supports both instant queries (`/api/v1/query`) and range queries (`/api/v1/query_range`).
*   Configurable server address, query string, and timeout via command-line flags.
*   Accepts start/end times for range queries in RFC3339 format or as Unix timestamps (integer or float).
*   Accepts step duration in Prometheus format (e.g., `15s`, `1m`, `1h`).
*   Uses the official Prometheus Go client library.
*   Prints the query, warnings, and results to the console.

## Usage

### Prerequisites

*   Go installed (version 1.18 or later recommended).
*   Access to a running Prometheus-compatible API endpoint.

### Building

```bash
# Navigate to the directory containing client.go
cd example/client

# Build the executable
go build -o query_client
```

### Running

Execute the client using flags to specify the target server and query details.

```bash
./query_client [flags]
```

### Flags

*   `-server <url>`: (Required) Full URL of the Prometheus compatible API server (e.g., `http://localhost:9090/api/v1` for Prometheus, `http://localhost:8080` for the default promql2mongo bridge).
*   `-query <string>`: (Required) The PromQL query to execute (e.g., `'http_requests_total{code="200"}'`). Remember to quote queries containing special characters.
*   `-timeout <int>`: Query timeout in seconds (default: `10`).
*   `-range`: Perform a range query instead of an instant query (default: `false`).
*   `-start <string>`: Start time for range query (RFC3339 or Unix timestamp). Required if `-range` is set.
*   `-end <string>`: End time for range query (RFC3339 or Unix timestamp). Required if `-range` is set.
*   `-step <duration>`: Step duration for range query (e.g., `15s`, `1m`, `1h`). (default: `1m`).

### Examples

**Instant Query:**

```bash
# Query the promql2mongo bridge (assuming default port 8080)
./query_client -server http://localhost:8080 -query 'http_requests_total{method="GET"}'

# Query a standard Prometheus server
./query_client -server http://localhost:9090/api/v1 -query 'node_cpu_seconds_total{mode="idle"}'
```

**Range Query:**

```bash
# Using RFC3339 timestamps against the bridge
./query_client -server http://localhost:8080 -range \
    -query 'node_memory_usage_bytes{instance="host-01.example.com"}' \
    -start '2025-04-23T10:00:00Z' \
    -end '2025-04-23T12:00:00Z' \
    -step '15m'

# Using Unix timestamps against the bridge
./query_client -server http://localhost:8080 -range \
    -query 'node_cpu_seconds_total{cpu_id="cpu0"}' \
    -start '1745373600' \
    -end '1745377200' \
    -step '60s'
```