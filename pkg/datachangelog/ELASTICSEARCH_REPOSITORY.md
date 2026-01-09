# Elasticsearch Repository Implementation

## Overview

The `ElasticsearchRepository` is a production-ready implementation of the `Repository` interface that uses Elasticsearch as the backend storage for audit logs. It provides high-performance, scalable storage and retrieval of data change logs with support for bulk operations, asynchronous writes, and complex queries.

## Factory Function

### `NewElasticsearchRepository(config *ElasticsearchConfig) (*ElasticsearchRepository, error)`

Creates and initializes a new Elasticsearch repository for storing and retrieving audit logs.

#### Parameters

- **config** (`*ElasticsearchConfig`): Configuration object containing:
  - `Enabled` (bool): Whether Elasticsearch is enabled
  - `Addresses` ([]string): List of Elasticsearch node addresses (e.g., `["https://localhost:9200"]`)
  - `Username` (string): Elasticsearch username for authentication
  - `Password` (string): Elasticsearch password for authentication
  - `APIKey` (string): Alternative to username/password for API key authentication
  - `InsecureSkipVerify` (bool): Skip TLS certificate verification (development only)
  - `CACert` (string): Path to CA certificate file
  - `IndexPrefix` (string): Prefix for index names (default: `"audit-log"`)
  - `IndexPattern` (string): Pattern for generating index names (e.g., `"{prefix}-{domain}-{yyyy.MM}"`)
  - `NumWorkers` (int): Number of async workers for bulk operations (default: 4)
  - `BulkSize` (int): Batch size for bulk operations (default: 100)
  - `MaxRetries` (int): Maximum retry attempts for failed operations (default: 3)
  - `RetryDelay` (time.Duration): Delay between retries (default: 500ms)
  - `FlushInterval` (time.Duration): Interval for flushing bulk queue (default: 2s)
  - `RequestTimeout` (time.Duration): Timeout for individual requests (default: 10s)

#### Returns

- **`*ElasticsearchRepository`**: Configured and initialized repository instance
- **`error`**: Non-nil if connection fails or configuration is invalid

#### Errors

The function returns errors in the following cases:

- Config is nil
- No Elasticsearch addresses specified
- Connection to Elasticsearch fails
- Health check fails
- Invalid authentication credentials

#### Example Usage

```go
package main

import (
	"context"
	"log"
	"time"

	"jecis.admission/infra/datachangelog"
)

func main() {
	// Create configuration
	config := &datachangelog.ElasticsearchConfig{
		Enabled:       true,
		Addresses:     []string{"https://localhost:9200"},
		Username:      "elastic",
		Password:      "your-password",
		IndexPrefix:   "audit-log",
		NumWorkers:    4,
		BulkSize:      100,
		MaxRetries:    3,
		FlushInterval: 2 * time.Second,
		RequestTimeout: 10 * time.Second,
	}

	// Create repository
	repo, err := datachangelog.NewElasticsearchRepository(config)
	if err != nil {
		log.Fatalf("Failed to create repository: %v", err)
	}
	defer repo.Close()

	// Use repository
	ctx := context.Background()
	
	// Check health
	if err := repo.Health(ctx); err != nil {
		log.Fatalf("Health check failed: %v", err)
	}

	// Save a log entry
	log := &datachangelog.DataChangeLog{
		Domain:          "appointment",
		Entity:          "Appointment",
		Operation:       "CREATE",
		PrimaryKeyStr:   "APT-001:BU-001",
		ChangedBy:       "user123",
		ChangedByEmail:  "user@example.com",
		ChangeTimestamp: time.Now(),
		IPAddress:       "192.168.1.100",
		UserAgent:       "Mozilla/5.0...",
	}

	if err := repo.Save(ctx, log); err != nil {
		log.Fatalf("Failed to save log: %v", err)
	}

	// Query logs
	query := &datachangelog.ChangeLogQuery{
		Domain:  "appointment",
		Entity:  "Appointment",
		Limit:   10,
		Offset:  0,
	}

	result, err := repo.Query(ctx, query)
	if err != nil {
		log.Fatalf("Failed to query logs: %v", err)
	}

	log.Printf("Found %d logs", result.Total)
}
```

## Key Features

### 1. Connection Management

- Automatic connection pooling and health checks
- Support for multiple Elasticsearch nodes (high availability)
- Configurable retry logic with exponential backoff
- TLS/SSL support with certificate validation

### 2. Index Management

- Dynamic index naming based on domain and date
- Pattern-based index creation (e.g., `audit-log-appointment-2024.01`)
- Automatic index lifecycle management
- Support for monthly index rotation

### 3. Bulk Operations

- Asynchronous bulk write processing
- Configurable batch size and flush intervals
- Automatic queue management
- Worker pool for parallel processing

### 4. Query Capabilities

- Complex filtering by domain, entity, primary key, operation, user, and date range
- Full text search support
- Aggregations for statistics
- Pagination support (limit/offset)

### 5. Performance Optimization

- Bulk API for batch writes
- Asynchronous processing with configurable workers
- Connection pooling
- Automatic refresh control

## Implementation Details

### Index Naming

Indices are created dynamically based on the configured pattern. Default pattern: `{prefix}-{domain}-{yyyy.MM}`

Example indices:
- `audit-log-appointment-2024.01`
- `audit-log-patient-2024.01`
- `audit-log-appointment-2024.02`

### Bulk Write Processing

The repository uses a `BulkIndexWriter` internally to handle asynchronous bulk operations:

1. Logs are queued as they arrive
2. When the queue reaches `BulkSize` or `FlushInterval` is reached, logs are flushed
3. Multiple workers process queues in parallel
4. Failed writes are logged and can be retried

### Query Building

Queries are automatically converted to Elasticsearch Query DSL with the following features:

- Term queries for exact matching (domain, entity, operation, user)
- Range queries for date filtering
- Wildcard index patterns for cross-index searches
- Sorting by `change_timestamp` in descending order

### Data Mapping

The repository expects the following Elasticsearch mapping for audit logs:

```json
{
  "mappings": {
    "properties": {
      "id": { "type": "keyword" },
      "domain": { "type": "keyword" },
      "entity": { "type": "keyword" },
      "operation": { "type": "keyword" },
      "primary_key": { "type": "object", "enabled": false },
      "primary_key_str": { "type": "keyword" },
      "before_data": { "type": "object", "enabled": false },
      "after_data": { "type": "object", "enabled": false },
      "changes": { "type": "nested" },
      "changed_by": { "type": "keyword" },
      "changed_by_email": { "type": "keyword" },
      "change_timestamp": { "type": "date" },
      "request_id": { "type": "keyword" },
      "ip_address": { "type": "ip" },
      "user_agent": { "type": "text" },
      "metadata": { "type": "object", "enabled": false }
    }
  }
}
```

## Methods

### Save Operations

#### `Save(ctx context.Context, log *DataChangeLog) error`

Saves a single audit log entry. Uses bulk writer if available for async processing.

```go
log := &datachangelog.DataChangeLog{
	Domain:    "appointment",
	Entity:    "Appointment",
	Operation: "CREATE",
	// ... other fields
}

if err := repo.Save(ctx, log); err != nil {
	log.Printf("Failed to save: %v", err)
}
```

#### `SaveBatch(ctx context.Context, logs []DataChangeLog) error`

Saves multiple logs in a single batch operation using bulk API.

```go
logs := []datachangelog.DataChangeLog{
	{/* log 1 */},
	{/* log 2 */},
	{/* log 3 */},
}

if err := repo.SaveBatch(ctx, logs); err != nil {
	log.Printf("Failed to save batch: %v", err)
}
```

### Query Operations

#### `Query(ctx context.Context, query *ChangeLogQuery) (*ChangeLogQueryResult, error)`

Executes a complex query against audit logs.

```go
query := &datachangelog.ChangeLogQuery{
	Domain:      "appointment",
	Entity:      "Appointment",
	Operation:   "UPDATE",
	StartDate:   time.Now().AddDate(0, 0, -7),
	EndDate:     time.Now(),
	Limit:       20,
	Offset:      0,
}

result, err := repo.Query(ctx, query)
if err != nil {
	log.Printf("Query failed: %v", err)
	return
}

for _, log := range result.Records {
	log.Printf("Log: %+v", log)
}
```

#### `GetByPrimaryKey(ctx context.Context, domain, entity, primaryKey string, limit, offset int) (*ChangeLogQueryResult, error)`

Retrieves all changes for a specific entity by primary key.

```go
result, err := repo.GetByPrimaryKey(ctx, "appointment", "Appointment", "APT-001:BU-001", 10, 0)
if err != nil {
	log.Printf("Failed: %v", err)
}
```

#### `GetEntityHistory(ctx context.Context, domain, entity, primaryKey string) (*EntityChangeHistory, error)`

Retrieves the complete change history for an entity.

```go
history, err := repo.GetEntityHistory(ctx, "appointment", "Appointment", "APT-001:BU-001")
if err != nil {
	log.Printf("Failed: %v", err)
	return
}

log.Printf("Change count: %d", history.ChangeCount)
log.Printf("First change: %v", history.FirstChange)
log.Printf("Last change: %v", history.LastChange)
log.Printf("Changed by: %v", history.ChangedByList)
```

### Maintenance Operations

#### `DeleteOlderThan(ctx context.Context, domain, entity string, date time.Time) error`

Deletes audit logs older than the specified date (useful for retention policies).

```go
// Delete logs older than 90 days
cutoffDate := time.Now().AddDate(0, 0, -90)
if err := repo.DeleteOlderThan(ctx, "appointment", "Appointment", cutoffDate); err != nil {
	log.Printf("Delete failed: %v", err)
}
```

#### `GetStats(ctx context.Context, domain, entity string, startDate, endDate time.Time) (*AuditStats, error)`

Returns statistics about audit logs for a given period.

```go
stats, err := repo.GetStats(
	ctx,
	"appointment",
	"Appointment",
	time.Now().AddDate(0, -1, 0),
	time.Now(),
)
if err != nil {
	log.Printf("Stats failed: %v", err)
	return
}

log.Printf("Total records: %d", stats.TotalRecords)
log.Printf("Unique users: %d", stats.UniqueUsers)
log.Printf("Operation counts: %v", stats.OperationCounts)
```

### Health and Lifecycle

#### `Health(ctx context.Context) error`

Checks if the Elasticsearch repository is healthy and accessible.

```go
if err := repo.Health(ctx); err != nil {
	log.Printf("Repository unhealthy: %v", err)
}
```

#### `Close() error`

Closes the repository connection and flushes any pending bulk operations.

```go
if err := repo.Close(); err != nil {
	log.Printf("Failed to close: %v", err)
}
```

## Configuration Examples

### Development Setup (Local Elasticsearch)

```go
config := &datachangelog.ElasticsearchConfig{
	Enabled:         true,
	Addresses:       []string{"http://localhost:9200"},
	Username:        "elastic",
	Password:        "changeme",
	IndexPrefix:     "audit-log",
	InsecureSkipVerify: true,
	NumWorkers:      2,
	BulkSize:        50,
	FlushInterval:   1 * time.Second,
}
```

### Production Setup (Cloud-hosted)

```go
config := &datachangelog.ElasticsearchConfig{
	Enabled:        true,
	Addresses:      []string{
		"https://es-node1.example.com:9200",
		"https://es-node2.example.com:9200",
		"https://es-node3.example.com:9200",
	},
	APIKey:         "my-api-key",
	IndexPrefix:    "audit-log",
	IndexPattern:   "{prefix}-{domain}-{yyyy.MM.dd}",
	NumWorkers:     8,
	BulkSize:       500,
	MaxRetries:     5,
	RetryDelay:     1 * time.Second,
	FlushInterval:  5 * time.Second,
	RequestTimeout: 30 * time.Second,
}
```

### High-Volume Production Setup

```go
config := &datachangelog.ElasticsearchConfig{
	Enabled:        true,
	Addresses:      []string{
		"https://es-node1.example.com:9200",
		"https://es-node2.example.com:9200",
		"https://es-node3.example.com:9200",
	},
	Username:       "audit_user",
	Password:       "secure-password",
	IndexPrefix:    "audit-log",
	IndexPattern:   "{prefix}-{domain}-{yyyy.MM.dd}",
	NumWorkers:     16,
	BulkSize:       1000,
	MaxRetries:     3,
	RetryDelay:     2 * time.Second,
	FlushInterval:  10 * time.Second,
	RequestTimeout: 60 * time.Second,
}
```

## Error Handling

The repository returns detailed errors for all operations:

```go
repo, err := datachangelog.NewElasticsearchRepository(config)
if err != nil {
	switch {
	case err.Error() == "elasticsearch config cannot be nil":
		// Handle nil config
	case strings.Contains(err.Error(), "failed to create elasticsearch client"):
		// Handle client creation error
	case strings.Contains(err.Error(), "health check failed"):
		// Handle connection error
	default:
		// Handle other errors
	}
}
```

## Performance Considerations

### Bulk Write Settings

- **BulkSize**: Larger values (500-1000) for high volume, smaller values (50-100) for low latency
- **NumWorkers**: Increase for high concurrency, decrease to reduce resource usage
- **FlushInterval**: Balance between latency and throughput

### Index Strategy

- Use monthly indices for better lifecycle management
- Daily indices for very high volume scenarios
- Yearly indices for compliance/archive scenarios

### Retention Policy

Implement automatic cleanup using `DeleteOlderThan`:

```go
// Daily cleanup job
ticker := time.NewTicker(24 * time.Hour)
defer ticker.Stop()

for range ticker.C {
	cutoff := time.Now().AddDate(0, 0, -90) // 90 day retention
	if err := repo.DeleteOlderThan(ctx, "appointment", "Appointment", cutoff); err != nil {
		log.Printf("Cleanup failed: %v", err)
	}
}
```

## Testing

The project also provides `NewMockElasticsearchRepository()` for testing:

```go
// For tests, use mock repository
mockRepo := datachangelog.NewMockElasticsearchRepository()
defer mockRepo.Close()

// Use mockRepo just like the real repository
// All operations are in-memory and don't require Elasticsearch
```

## Security Considerations

1. **Authentication**: Use API keys in production instead of username/password
2. **TLS**: Always use HTTPS in production (`https://` addresses)
3. **Certificate Validation**: Set `InsecureSkipVerify: false` in production
4. **Sensitive Data**: Configure sanitizers for PII fields in the YAML config
5. **Access Control**: Use Elasticsearch RBAC to restrict audit log access

## Troubleshooting

### Connection Issues

```go
// Test connectivity
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := repo.Health(ctx); err != nil {
	log.Printf("Connection failed: %v", err)
	// Check: addresses, credentials, network, TLS certificates
}
```

### Slow Queries

```go
// Check statistics to understand query patterns
stats, _ := repo.GetStats(ctx, "appointment", "Appointment", start, end)
log.Printf("Stats: %+v", stats)
// Consider adding indices or adjusting query filters
```

### Bulk Write Failures

```go
status := repo.bulkWriter.Status()
log.Printf("Queue size: %d", status.QueueSize)
log.Printf("Failed count: %d", status.FailedCount)
log.Printf("Processed count: %d", status.ProcessedCount)
```

## Dependencies

- `github.com/elastic/go-elasticsearch/v8`: Official Elasticsearch Go client
- `github.com/google/uuid`: For generating unique log IDs

## Migration from Mock to Production

Simply replace the initialization:

```go
// Before (mock)
repo := datachangelog.NewMockElasticsearchRepository()

// After (production)
config := &datachangelog.ElasticsearchConfig{
	Enabled:   true,
	Addresses: []string{"https://localhost:9200"},
	// ... other config
}
repo, err := datachangelog.NewElasticsearchRepository(config)
```

All other code remains the same due to the common `Repository` interface.