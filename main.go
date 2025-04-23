package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql/parser"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Host      string `yaml:"host"`
		Port      int    `yaml:"port"`
		QueryPath string `yaml:"queryPath"`
	} `yaml:"server"`
	MongoDB struct {
		URI      string `yaml:"uri"`
		Database string `yaml:"database"`
		Timeout  int    `yaml:"timeout"`
	} `yaml:"mongodb"`
	Collections map[string]CollectionInfo `yaml:"collections"`
	Mappings    map[string]string         `yaml:"mappings"`
}

var (
	conf   Config
	client *mongo.Client
)

func main() {
	configFile := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	f, err := os.Open(*configFile)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := yaml.NewDecoder(f).Decode(&conf); err != nil {
		log.Fatal(err)
	}

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(conf.MongoDB.Timeout)*time.Second)
	defer cancel()
	client, err = mongo.Connect(ctx, options.Client().ApplyURI(conf.MongoDB.URI))
	if err != nil {
		log.Fatal(err)
	}

	// Set up server
	http.HandleFunc(conf.Server.QueryPath, handleQuery)
	addr := fmt.Sprintf("%s:%d", conf.Server.Host, conf.Server.Port)
	log.Printf("Server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// parseTime parses a Prometheus timestamp string (Unix seconds or RFC3339)
func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty time string")
	}
	// Try parsing as Unix timestamp (float or int)
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		// Handle potential fractional seconds
		secs := int64(f)
		nsecs := int64((f - float64(secs)) * 1e9)
		return time.Unix(secs, nsecs), nil
	}
	// Try parsing as RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse %q: invalid format", s)
}

// parseDuration parses a Prometheus duration string (like "5m", "1h")
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, errors.New("empty duration string")
	}
	// Try parsing as float seconds first
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(f * float64(time.Second)), nil
	}
	// Try parsing using Prometheus model Duration
	if d, err := model.ParseDuration(s); err == nil {
		return time.Duration(d), nil
	}
	return 0, fmt.Errorf("cannot parse %q: invalid format", s)
}

func handleQuery(w http.ResponseWriter, r *http.Request) {
	// Get query parameter from URL query parameters
	queryValues := r.URL.Query()
	queryParam := queryValues.Get("query")
	startParam := queryValues.Get("start")
	endParam := queryValues.Get("end")
	stepParam := queryValues.Get("step")

	isRangeQuery := startParam != "" && endParam != "" && stepParam != ""
	var startTime, endTime time.Time
	var step time.Duration
	var err error

	if isRangeQuery {
		startTime, err = parseTime(startParam)
		if err != nil {
			sendJSONError(w, http.StatusBadRequest, "bad_data", fmt.Sprintf("invalid start time: %v", err))
			return
		}
		endTime, err = parseTime(endParam)
		if err != nil {
			sendJSONError(w, http.StatusBadRequest, "bad_data", fmt.Sprintf("invalid end time: %v", err))
			return
		}
		step, err = parseDuration(stepParam)
		if err != nil {
			sendJSONError(w, http.StatusBadRequest, "bad_data", fmt.Sprintf("invalid step: %v", err))
			return
		}
		if step <= 0 {
			sendJSONError(w, http.StatusBadRequest, "bad_data", "zero or negative step")
			return
		}
		if endTime.Before(startTime) {
			sendJSONError(w, http.StatusBadRequest, "bad_data", "end time must not be before start time")
			return
		}
		log.Printf("Debug: Range query detected: start=%v, end=%v, step=%v", startTime, endTime, step)
	} else {
		log.Printf("Debug: Instant query detected")
	}

	// If query is empty and it's a POST request, try to read from body
	if queryParam == "" && r.Method == "POST" {
		// First try to parse form data - do this BEFORE reading the body
		err := r.ParseForm()
		if err == nil {
			queryParam = r.PostFormValue("query")
			log.Printf("Debug: found query in form: %s", queryParam)
		}

		// If still empty, try JSON body
		if queryParam == "" && r.Body != nil {
			bodyBytes, err := io.ReadAll(r.Body)
			if err == nil && len(bodyBytes) > 0 {
				log.Printf("Debug: received POST body: %s", string(bodyBytes))

				// Try to parse as JSON
				var jsonData map[string]interface{}
				if json.Unmarshal(bodyBytes, &jsonData) == nil {
					log.Printf("Debug: found JSON data: %v", jsonData)
					if query, ok := jsonData["query"].(string); ok && query != "" {
						queryParam = query
					}
				}

				// Restore the body for other handlers
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}
		}
	}
	// Handle empty query
	if queryParam == "" {
		sendJSONError(w, http.StatusBadRequest, "bad_data", "empty query parameter")
		return
	}

	metric, labels, err := parsePromQL(queryParam)
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	collKey, ok := conf.Mappings[metric]
	if !ok {
		sendJSONError(w, http.StatusBadRequest, "bad_data", "unknown metric")
		return
	}

	collInfo := conf.Collections[collKey]
	// Pass time range to buildMongoFilter if it's a range query
	filter := buildMongoFilter(labels, collInfo.LabelFields, collInfo.TimeField, startTime, endTime)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cursor, err := client.Database(conf.MongoDB.Database).Collection(collInfo.Name).Find(ctx, filter)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	defer cursor.Close(ctx)

	// Pass isRangeQuery flag to mongoCursorToProm
	results, err := mongoCursorToProm(cursor, collInfo, isRangeQuery)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		// Log error, but response might be already partially written
		log.Printf("Error encoding JSON response: %v", err)
		// Avoid calling sendJSONError here as headers might be sent
	}
}

// sendJSONError writes a JSON-formatted error response that the Prometheus client can parse.
func sendJSONError(w http.ResponseWriter, status int, errorType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]interface{}{
		"status":    "error",
		"errorType": errorType,
		"error":     message,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func parsePromQL(query string) (string, map[string]string, error) {
	log.Default().Printf("Debug: parsing query: %s", query)
	expr, err := parser.ParseExpr(query)
	if err != nil {
		return "", nil, err
	}

	labelMap := map[string]string{}
	var metricName string

	// Identify if expression is a VectorSelector or MatrixSelector
	var vs *parser.VectorSelector
	switch e := expr.(type) {
	case *parser.VectorSelector:
		vs = e
	case *parser.MatrixSelector:
		vs = e.VectorSelector.(*parser.VectorSelector)
	default:
		return "", nil, fmt.Errorf("unsupported expression type: %T", expr)
	}

	// Pull out __name__ (the metric name) and any other labels
	for _, matcher := range vs.LabelMatchers {
		if matcher.Name == model.MetricNameLabel { // "__name__"
			metricName = matcher.Value
		} else {
			labelMap[matcher.Name] = matcher.Value
		}
	}
	return metricName, labelMap, nil
}

// Add timeField, startTime, endTime parameters
func buildMongoFilter(labels map[string]string, fields map[string]string, timeField string, startTime, endTime time.Time) map[string]interface{} {
	filter := make(map[string]interface{})
	for k, v := range labels {
		if mappedField, ok := fields[k]; ok {
			filter[mappedField] = v
		}
	}
	// Add time range filter if start and end times are provided (non-zero)
	if !startTime.IsZero() && !endTime.IsZero() && timeField != "" {
		filter[timeField] = map[string]interface{}{
			"$gte": startTime,
			"$lte": endTime,
		}
	}
	return filter
}

// Define a named type for collection info to avoid type mismatch
type CollectionInfo struct {
	Name        string            `yaml:"name"`
	TimeField   string            `yaml:"timeField"`
	MetricField string            `yaml:"metricField"` // Field for __name__ label
	ValueField  string            `yaml:"valueField"`  // Field for the numeric value
	LabelFields map[string]string `yaml:"labelFields"`
	DefaultLbls map[string]string `yaml:"defaultLabels"`
}

func mongoCursorToProm(cursor *mongo.Cursor, colInfo CollectionInfo, isRangeQuery bool) (map[string]interface{}, error) {
	resp := map[string]interface{}{
		"status": "success",
		"data":   map[string]interface{}{},
	}

	if isRangeQuery {
		// --- Range Query Logic (Matrix) ---
		resp["data"].(map[string]interface{})["resultType"] = "matrix"
		// Group results by metric signature (labels)
		seriesMap := make(map[string]map[string]interface{}) // Map: label_signature -> series_data

		ctx := context.TODO()
		for cursor.Next(ctx) {
			var doc map[string]interface{}
			if err := cursor.Decode(&doc); err != nil {
				log.Printf("Error decoding document: %v", err)
				continue // Skip problematic document
			}

			timestamp, valueStr, metricLabels, err := extractDataFromDoc(doc, colInfo)
			if err != nil {
				log.Printf("Error extracting data from doc: %v", err)
				continue
			}

			// Create a unique signature for the series based on labels
			labelSignature := createLabelSignature(metricLabels)

			// Find or create the series entry
			series, exists := seriesMap[labelSignature]
			if !exists {
				series = map[string]interface{}{
					"metric": metricLabels,
					"values": make([]interface{}, 0), // Initialize as empty slice
				}
				seriesMap[labelSignature] = series
			}

			// Append the value [timestamp, valueStr]
			values := series["values"].([]interface{})
			series["values"] = append(values, []interface{}{timestamp, valueStr})
		}
		if err := cursor.Err(); err != nil {
			return nil, fmt.Errorf("cursor error: %w", err)
		}

		// Convert map to slice for final result
		matrixResult := make([]interface{}, 0, len(seriesMap))
		for _, series := range seriesMap {
			matrixResult = append(matrixResult, series)
		}
		resp["data"].(map[string]interface{})["result"] = matrixResult

	} else {
		// --- Instant Query Logic (Vector) ---
		resp["data"].(map[string]interface{})["resultType"] = "vector"
		vectorResult := make([]interface{}, 0) // Always use an empty slice

		ctx := context.TODO()
		// For instant queries, we typically want the *latest* point for each series.
		// Process all points and then filter to get the latest point for each unique set of labels.
		// this is naively done by using a map to track the latest point for each label set.
		// This approach need to be optimized as it is .. not great for performance.
		// for example different values for the same lableset and timestamp are not handled (only the latest one is kept)
		latestPoints := make(map[string]map[string]interface{}) // Map: label_signature -> latest_sample
		for cursor.Next(ctx) {
			var doc map[string]interface{}
			if err := cursor.Decode(&doc); err != nil {
				log.Printf("Error decoding document: %v", err)
				continue
			}

			timestamp, valueStr, metricLabels, err := extractDataFromDoc(doc, colInfo)
			if err != nil {
				log.Printf("Error extracting data from doc: %v", err)
				continue
			}

			// Create a unique signature for the series based on labels
			labelSignature := createLabelSignature(metricLabels)

			// Check if we already have a point for this label set
			existing, exists := latestPoints[labelSignature]
			if !exists || timestamp > existing["value"].([]interface{})[0].(float64) {
				// Store this as the latest point for this label set
				latestPoints[labelSignature] = map[string]interface{}{
					"metric": metricLabels,
					"value":  []interface{}{timestamp, valueStr},
				}
			}
		}

		if err := cursor.Err(); err != nil {
			return nil, fmt.Errorf("cursor error: %w", err)
		}

		// Convert map to slice for final result
		for _, sample := range latestPoints {
			vectorResult = append(vectorResult, sample)
		}

		resp["data"].(map[string]interface{})["result"] = vectorResult
	}

	return resp, nil
}

// Helper function to extract data and labels from a MongoDB document
func extractDataFromDoc(doc map[string]interface{}, colInfo CollectionInfo) (float64, string, map[string]string, error) {
	// Extract timestamp
	var timestamp float64
	if timeVal, ok := doc[colInfo.TimeField]; ok {
		switch tv := timeVal.(type) {
		case time.Time:
			// Convert to Unix timestamp with potential fractions of a second
			timestamp = float64(tv.UnixNano()) / 1e9
		case string:
			if t, err := time.Parse(time.RFC3339Nano, tv); err == nil { // Try Nano first
				timestamp = float64(t.UnixNano()) / 1e9
			} else if t, err := time.Parse(time.RFC3339, tv); err == nil { // Fallback to RFC3339
				timestamp = float64(t.UnixNano()) / 1e9
			} else {
				log.Printf("Warning: could not parse time string '%s', using current time", tv)
				timestamp = float64(time.Now().UnixNano()) / 1e9
			}
		case float64:
			timestamp = tv // Assume it's already Unix seconds
		case int64:
			timestamp = float64(tv) // Assume it's Unix seconds
		case int32:
			timestamp = float64(tv) // Assume it's Unix seconds
		// Add handling for MongoDB specific date types if necessary (e.g., primitive.DateTime)
		default:
			log.Printf("Warning: unhandled time type '%T' for field '%s', using current time", tv, colInfo.TimeField)
			timestamp = float64(time.Now().UnixNano()) / 1e9
		}
	} else {
		log.Printf("Warning: time field '%s' not found, using current time", colInfo.TimeField)
		timestamp = float64(time.Now().UnixNano()) / 1e9
	}

	// --- Extract numeric metric value from ValueField ---
	metricValueStr := "0" // Default value
	if val, ok := doc[colInfo.ValueField]; ok {
		switch v := val.(type) {
		case float64, float32, int, int64, int32:
			metricValueStr = fmt.Sprintf("%v", v)
		case string:
			// Validate if it looks like a number before using it
			if _, err := strconv.ParseFloat(v, 64); err == nil {
				metricValueStr = v
			} else {
				log.Printf("Warning: non-numeric string value '%v' found in ValueField '%s', using default '0'", v, colInfo.ValueField)
			}
		default:
			strVal := fmt.Sprintf("%v", v)
			if _, err := strconv.ParseFloat(strVal, 64); err == nil {
				metricValueStr = strVal
			} else {
				log.Printf("Warning: unparseable value type '%T' ('%v') in ValueField '%s', using default '0'", v, v, colInfo.ValueField)
			}
		}
	} else {
		log.Printf("Warning: value field '%s' not found, using default '0'", colInfo.ValueField)
	}
	// ----------------------------------------------------

	// Build metric labels
	metricLabels := make(map[string]string)
	// Add default labels first
	for k, v := range colInfo.DefaultLbls {
		metricLabels[k] = v
	}
	// Add labels from the document, potentially overwriting defaults
	for promLabel, mongoField := range colInfo.LabelFields {
		if val, ok := doc[mongoField]; ok {
			metricLabels[promLabel] = fmt.Sprintf("%v", val) // Convert label value to string
		}
	}

	// --- Add __name__ label based on the MetricField value ---
	if nameVal, ok := doc[colInfo.MetricField]; ok {
		metricLabels[model.MetricNameLabel] = fmt.Sprintf("%v", nameVal)
	} else if _, ok := metricLabels[model.MetricNameLabel]; !ok {
		// If __name__ wasn't set by defaults or labels, and MetricField was missing, log a warning.
		log.Printf("Warning: MetricField '%s' not found and no default __name__ label set.", colInfo.MetricField)
		// Optionally set a default __name__ here if desired, e.g.:
		// metricLabels[model.MetricNameLabel] = "unknown"
	}
	// ---------------------------------------------------------

	// Return timestamp, the numeric metric value string, the labels map, and nil error
	return timestamp, metricValueStr, metricLabels, nil
}

// Helper function to create a unique string signature from labels for grouping
func createLabelSignature(labels map[string]string) string {
	// A simple approach is to marshal the map to JSON.
	// For guaranteed order, keys first must be sorted (more complex). JSON is usually sufficient.
	bytes, err := json.Marshal(labels)
	if err != nil {
		// Fallback or handle error - shouldn't happen with map[string]string
		log.Printf("Error creating label signature: %v", err)
		return fmt.Sprintf("%v", labels) // Less reliable fallback
	}
	return string(bytes)
}
