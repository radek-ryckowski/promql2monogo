package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

func main() {
	// Define command line flags
	serverAddress := flag.String("server", "http://localhost:9090", "Prometheus compatible API server address ")
	promQuery := flag.String("query", "my_metric{label1=\"value1\"}", "PromQL query to execute")
	timeout := flag.Int("timeout", 10, "Query timeout in seconds")
	isRangeQuery := flag.Bool("range", false, "Perform a range query instead of an instant query")
	startTimeStr := flag.String("start", "", "Start time for range query (RFC3339 or Unix timestamp)")
	endTimeStr := flag.String("end", "", "End time for range query (RFC3339 or Unix timestamp)")
	stepStr := flag.String("step", "1m", "Step duration for range query (e.g., '15s', '1m', '1h')")

	flag.Parse()

	// Validate flags for range query
	if *isRangeQuery && (*startTimeStr == "" || *endTimeStr == "") {
		fmt.Println("Error: --start and --end flags are required for range queries (--range)")
		os.Exit(1)
	}

	// Create Prometheus API client with configurable address
	client, err := api.NewClient(api.Config{
		Address: *serverAddress,
	})
	if err != nil {
		log.Fatalf("Error creating client: %v\n", err)
	}

	v1api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	fmt.Printf("Connecting to: %s\n", *serverAddress)

	if *isRangeQuery {
		// --- Range Query ---
		startTime, err := parseTimeInput(*startTimeStr)
		if err != nil {
			log.Fatalf("Error parsing start time: %v\n", err)
		}
		endTime, err := parseTimeInput(*endTimeStr)
		if err != nil {
			log.Fatalf("Error parsing end time: %v\n", err)
		}
		step, err := model.ParseDuration(*stepStr)
		if err != nil {
			log.Fatalf("Error parsing step duration: %v\n", err)
		}

		queryRange := v1.Range{
			Start: startTime,
			End:   endTime,
			Step:  time.Duration(step),
		}

		fmt.Printf("Sending range query: %s\n", *promQuery)
		fmt.Printf("Range: Start=%v, End=%v, Step=%v\n", queryRange.Start, queryRange.End, queryRange.Step)

		result, warnings, err := v1api.QueryRange(ctx, *promQuery, queryRange)
		if err != nil {
			log.Fatalf("Range query error: %v\n", err)
		}
		if len(warnings) > 0 {
			fmt.Printf("Warnings: %v\n", warnings)
		}
		fmt.Printf("Query: %s\n", *promQuery)
		fmt.Printf("Result:\n%v\n", result)

	} else {
		// --- Instant Query ---
		fmt.Printf("Sending instant query: %s\n", *promQuery)
		result, warnings, err := v1api.Query(ctx, *promQuery, time.Now()) // Use time.Now() for instant query
		if err != nil {
			log.Fatalf("Instant query error: %v\n", err)
		}
		if len(warnings) > 0 {
			fmt.Printf("Warnings: %v\n", warnings)
		}
		fmt.Printf("Query: %s\n", *promQuery)
		fmt.Printf("Result:\n%v\n", result)
	}
}

// Helper function to parse time strings (RFC3339 or Unix timestamp)
func parseTimeInput(timeStr string) (time.Time, error) {
	// Try parsing as RFC3339
	t, err := time.Parse(time.RFC3339, timeStr)
	if err == nil {
		return t, nil
	}
	// Try parsing as Unix timestamp
	unixTime, err := parseUnixTimestamp(timeStr)
	if err == nil {
		return unixTime, nil
	}
	return time.Time{}, fmt.Errorf("invalid time format: %q. Use RFC3339 or Unix timestamp", timeStr)
}

// Helper function to parse Unix timestamp (integer or float)
func parseUnixTimestamp(tsStr string) (time.Time, error) {
	// Try parsing as float first (to handle potential fractions)
	if f, err := strconv.ParseFloat(tsStr, 64); err == nil {
		secs := int64(f)
		nsecs := int64((f - float64(secs)) * 1e9)
		return time.Unix(secs, nsecs), nil
	}
	// Try parsing as integer
	if i, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
		return time.Unix(i, 0), nil
	}
	return time.Time{}, fmt.Errorf("not a valid Unix timestamp: %s", tsStr)
}
