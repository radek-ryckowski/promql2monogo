package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Define command line flags
	mongoURL := flag.String("url", "mongodb://localhost:27017", "MongoDB connection URL")
	flag.Parse()

	// Log the connection URL being used
	log.Printf("Connecting to MongoDB at: %s", *mongoURL)

	// Connect to MongoDB using the URL from the flag
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(*mongoURL))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(context.Background())

	// Generate data for "metrics_http" collection
	generateHttpMetrics(client)

	// Generate data for "metrics_system" collection
	generateCpuMetrics(client)

	// Generate data for "metrics_memory" collection
	generateMemoryMetrics(client)

	fmt.Println("Sample data generated for all collections.")
}

func generateHttpMetrics(client *mongo.Client) {
	fmt.Println("Generating HTTP metrics...")
	collHttp := client.Database("metrics_db").Collection("metrics_http")

	// Clear existing data
	collHttp.DeleteMany(context.Background(), bson.M{})

	// Generate some HTTP request metrics
	for i := 0; i < 20; i++ {
		statusCodes := []int{200, 200, 200, 400, 500} // Make 200s more common
		methods := []string{"GET", "POST", "PUT", "DELETE"}
		endpoints := []string{"/api/users", "/api/products", "/api/orders", "/health"}
		serverIDs := []string{"instance-1", "instance-2"}

		// Create documents with multiple metric types
		metricName := "http_requests_total"
		if i%3 == 0 { // Mix in some duration metrics
			metricName = "http_request_duration_seconds"
		}

		doc := bson.M{
			"timestamp":   time.Now().Add(time.Duration(-i) * time.Minute),
			"metric_name": metricName,
			"value":       rand.Float64() * 1000, // Numeric value for the metric
			"status_code": statusCodes[rand.Intn(len(statusCodes))],
			"http_method": methods[rand.Intn(len(methods))],
			"endpoint":    endpoints[rand.Intn(len(endpoints))],
			"server_id":   serverIDs[rand.Intn(len(serverIDs))],
		}

		_, err := collHttp.InsertOne(context.Background(), doc)
		if err != nil {
			log.Println("Insert error:", err)
		}
	}
	fmt.Println("HTTP metrics generated.")
}

func generateCpuMetrics(client *mongo.Client) {
	fmt.Println("Generating CPU metrics...")
	collSystem := client.Database("metrics_db").Collection("metrics_system")

	// Clear existing data
	collSystem.DeleteMany(context.Background(), bson.M{})

	// Generate some CPU usage metrics
	cpuModes := []string{"user", "system", "idle", "iowait", "irq", "nice"}
	hosts := []string{"host-01.example.com", "host-02.example.com"}

	for i := 0; i < 30; i++ {
		for cpuID := 0; cpuID < 4; cpuID++ { // Generate data for 4 CPUs
			for _, mode := range cpuModes {
				// CPU seconds values depend on the mode
				value := 0.0
				switch mode {
				case "idle":
					value = rand.Float64() * 70 // Higher idle time
				case "user":
					value = rand.Float64() * 20
				case "system":
					value = rand.Float64() * 10
				default:
					value = rand.Float64() * 5
				}

				doc := bson.M{
					"ts":       time.Now().Add(time.Duration(-i) * time.Minute),
					"name":     "node_cpu_seconds_total",
					"value":    value,
					"cpu_mode": mode,
					"cpu_id":   fmt.Sprintf("cpu%d", cpuID),
					"host":     hosts[rand.Intn(len(hosts))],
				}

				_, err := collSystem.InsertOne(context.Background(), doc)
				if err != nil {
					log.Println("Insert error:", err)
				}
			}
		}
	}
	fmt.Println("CPU metrics generated.")
}

func generateMemoryMetrics(client *mongo.Client) {
	fmt.Println("Generating memory metrics...")
	collMemory := client.Database("metrics_db").Collection("metrics_memory")

	// Clear existing data
	collMemory.DeleteMany(context.Background(), bson.M{})

	// Generate some memory usage metrics
	memoryTypes := []string{"used", "free", "cached", "buffers", "available"}
	hosts := []string{"host-01.example.com", "host-02.example.com", "host-03.example.com"}

	for i := 0; i < 24; i++ { // Generate 24 hours of data
		for _, host := range hosts {
			// Create a baseline for each host that changes slightly over time
			baseTotal := 16.0 * 1024 * 1024 * 1024                // 16GB in bytes
			baseUsed := (4.0 + float64(i%5)) * 1024 * 1024 * 1024 // 4-8GB used, varies over time

			for _, memType := range memoryTypes {
				// Calculate value based on memory type and baseline
				value := 0.0
				switch memType {
				case "used":
					value = baseUsed
				case "free":
					value = baseTotal - baseUsed
				case "cached":
					value = float64(rand.Intn(2048)) * 1024 * 1024 // 0-2GB cached
				case "buffers":
					value = float64(rand.Intn(512)) * 1024 * 1024 // 0-512MB buffers
				case "available":
					value = baseTotal - (baseUsed * 0.7) // Some used memory can be reclaimed
				}

				doc := bson.M{
					"time":        time.Now().Add(time.Duration(-i) * time.Hour),
					"metric":      "node_memory_usage_bytes",
					"value":       value,
					"memory_type": memType,
					"host_id":     host,
				}

				_, err := collMemory.InsertOne(context.Background(), doc)
				if err != nil {
					log.Println("Insert error:", err)
				}
			}
		}
	}
	fmt.Println("Memory metrics generated.")
}
