# PromQL to MongoDB Bridge

This program acts as a simple bridge allowing Prometheus to query time-series data stored in MongoDB. It exposes an HTTP endpoint compatible with the Prometheus remote read API (`/api/v1/query` and `/api/v1/query_range`) and translates basic PromQL queries into MongoDB find operations.

## Functionality

*   Listens for HTTP requests on a configurable host, port, and path.
*   Connects to a specified MongoDB instance and database.
*   Parses simple PromQL queries (metric name and label selectors).
*   Uses a configuration file (`config.yaml`) to map Prometheus metric names and labels to MongoDB collection names and fields.
*   Supports both instant queries (`/api/v1/query`) and range queries (`/api/v1/query_range`).
*   Formats MongoDB results into the Prometheus remote read JSON format (`vector` or `matrix`).

## Configuration

Configuration is managed via `config.yaml`. See the example file for details on setting up server parameters, MongoDB connection details, collection mappings, and label mappings.

## Limitations

This bridge is designed for simple use cases and has several limitations:

*   **Basic Queries Only:** Only supports queries consisting of a metric name and simple label equality matchers (e.g., `my_metric{label1="value1", label2="value2"}`).
*   **No Regex Matching:** Label matchers using regular expressions (`=~`, `!~`) are not supported.
*   **No Complex PromQL Functions/Operators:** Functions (like `rate()`, `sum()`, `avg()`), aggregations, binary operators, subqueries, and offset modifiers are **not** supported. The query must directly map to selecting documents based on labels.
*   **No Step Interpolation:** For range queries, it returns all data points found within the `start` and `end` timestamps. It does not perform interpolation or alignment based on the `step` parameter.
*   **Limited Error Handling:** While basic error responses are provided, complex query errors might not be gracefully handled.
*   **Performance:** Performance depends heavily on MongoDB indexing for the queried label fields and the time field. Large range queries or queries returning many series might be slow.