# Audit Data Change Log Middleware

A comprehensive gRPC middleware for logging and auditing all data changes in the JECIS Admission system. This middleware captures before/after snapshots, calculates diffs, sanitizes sensitive data, and persists audit logs to Elasticsearch.

## Features

- **Comprehensive Audit Logging**: Captures all CREATE, UPDATE, and DELETE operations
- **Before/After Snapshots**: Optional full data snapshots for complete change history
- **Diff Calculation**: Automatically computes field-level changes with before/after values
- **Sensitive Data Protection**: Automatically redacts/masks sensitive fields (passwords, emails, phone numbers, SSN, etc.)
- **Flexible Primary Key Support**: Supports single or composite primary keys
- **Elasticsearch Integration**: Stores logs in Elasticsearch with automatic index rotation by domain and month
- **Async Processing**: Non-blocking batch processing with configurable worker pools
- **Field-Level Exclusion**: Skip logging specific fields (internal flags, timestamps, etc.)
- **Custom Transformers**: Apply custom transformations to specific fields
- **User Tracking**: Captures user ID, email, IP address, and user agent for audit trail
- **Compliance Ready**: Designed to meet audit and compliance requirements

## Installation

The middleware is located in `infra/datachangelog/` package.

### Add to go.mod

The module requires the Elasticsearch client (add to go.mod if using real Elasticsearch):

```bash
go get github.com/elastic/go-elasticsearch/v8
```

## Quick Start

### 1. Configure Audit Logging

Create/update `config/datachangelog_config.yaml`:

```yaml
elasticsearch:
  enabled: true
  addresses:
    - "http://localhost:9200"
  username: "elastic"
  password: "password"
  index_prefix: "audit-log"
  index_pattern: "{prefix}-{domain}-{yyyy.MM}"
  num_workers: 4
  bulk_size: 100

global:
  enabled: true
  sensitive_fields:
    - "password"
    - "email"
    - "phone"
    - "ssn"

entities:
  - domain: "appointment"
    entity: "Appointment"
    enabled: true
    operations: ["CREATE", "UPDATE", "DELETE"]
    primary_key:
      single_key: "id"
    excluded_fields: ["created_at", "updated_at"]
    sensitive_fields: ["patient_phone", "patient_email"]
    include_before_data: true
    include_after_data: true
```

### 2. Initialize in main.go

```go
package main

import (
	"os"
	"jecis.admission/infra/datachangelog"
)

func main() {
	// ... existing setup code ...

	// Load audit configuration
	configData, err := os.ReadFile("config/datachangelog_config.yaml")
	if err != nil {
		log.Fatalf("Failed to load audit config: %v", err)
	}

	auditConfig, err := datachangelog.LoadConfig(configData)
	if err != nil {
		log.Fatalf("Invalid audit config: %v", err)
	}

	// Create repository (use MockElasticsearchRepository for testing)
	var auditRepo datachangelog.Repository
	if auditConfig.Elasticsearch.Enabled {
		// For production, implement real Elasticsearch repository
		// For now, use mock:
		auditRepo = datachangelog.NewMockElasticsearchRepository()
	}

	// Create interceptor configuration
	interceptorCfg := &datachangelog.InterceptorConfig{
		Enabled:           true,
		Config:            auditConfig,
		Repository:        auditRepo,
		Sanitizer:         datachangelog.NewSanitizer(auditConfig.Global.SensitiveFields),
		DiffCalculator:    datachangelog.NewDiffCalculator(auditConfig.Global.ExcludedFields, auditConfig.Global.SensitiveFields),
		CaptureBeforeData: true,
		CaptureAfterData:  true,
		UserExtractor:     &datachangelog.DefaultUserExtractor{},
		IPExtractor:       &datachangelog.DefaultIPExtractor{},
	}

	// Add interceptor to gRPC chain
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			nrgrpc.UnaryServerInterceptor(appNewRelic),
			infra_grpc.EnsureValidToken,
			customvalidator.GrpcErrorHandler(),
			datachangelog.NewAuditInterceptor(interceptorCfg),
			recovery.UnaryServerInterceptor(
				recovery.WithRecoveryHandlerContext(grpcPanicRecoveryHandler),
			),
		),
		grpc.StreamInterceptor(nrgrpc.StreamServerInterceptor(appNewRelic)),
		grpc.Creds(credentials.NewServerTLSFromCert(&cert)),
	)

	// ... rest of your setup ...
}
```

## Configuration Guide

### Elasticsearch Section

- **enabled**: Enable/disable audit logging
- **addresses**: Elasticsearch server addresses (e.g., `["http://localhost:9200"]`)
- **username/password**: Authentication credentials
- **index_prefix**: Prefix for all indices (e.g., "audit-log")
- **index_pattern**: Index naming pattern with placeholders:
  - `{prefix}`: Index prefix
  - `{domain}`: Domain name
  - `{yyyy}`: 4-digit year
  - `{MM}`: 2-digit month
  - `{dd}`: 2-digit day
- **num_workers**: Number of async workers (default: 4)
- **bulk_size**: Batch size for bulk operations (default: 100)
- **max_retries**: Number of retries on failure (default: 3)
- **retry_delay**: Delay between retries
- **flush_interval**: How often to flush pending logs
- **request_timeout**: HTTP request timeout

### Global Section

- **enabled**: Enable/disable all audit logging
- **excluded_fields**: Fields to exclude from all entities (e.g., timestamps)
- **sensitive_fields**: Fields to redact in all entities
- **include_before_data**: Include full before snapshot
- **include_after_data**: Include full after snapshot
- **include_ip_address**: Capture client IP address
- **include_user_agent**: Capture client user agent
- **max_metadata_size**: Maximum size for metadata storage

### Entities Section

Each entity requires:

- **domain**: Domain name (e.g., "appointment")
- **entity**: Entity name (e.g., "Appointment")
- **enabled**: Enable/disable this entity's logging
- **operations**: List of operations to log (CREATE, UPDATE, DELETE)
- **primary_key**: Primary key configuration:
  - **single_key**: Single field name
  - **composite_keys**: Array of field names for composite keys
  - **separator**: Separator for composite keys (default: ":")
- **excluded_fields**: Fields to exclude for this entity
- **sensitive_fields**: Fields to redact for this entity
- **include_before_data**: Override global setting for before snapshots
- **include_after_data**: Override global setting for after snapshots
- **transformers**: Custom transformers for specific fields
- **metadata**: Custom metadata to include in all logs

## Usage Examples

### Querying Audit Logs

```go
// Get all changes for a specific appointment
query := &datachangelog.ChangeLogQuery{
	Domain:        "appointment",
	Entity:        "Appointment",
	PrimaryKeyStr: "123",
	Limit:         100,
	Offset:        0,
}

result, err := auditRepo.Query(context.Background(), query)
if err != nil {
	log.Fatal(err)
}

for _, log := range result.Records {
	fmt.Printf("User %s made %s on %v\n", log.ChangedBy, log.Operation, log.ChangeTimestamp)
	for _, diff := range log.Changes {
		fmt.Printf("  Field %s: %v -> %v\n", diff.FieldName, diff.OldValue, diff.NewValue)
	}
}
```

### Getting Entity History

```go
// Get complete change history for a patient
history, err := auditRepo.GetEntityHistory(context.Background(), "patient", "Patient", "patient-123")
if err != nil {
	log.Fatal(err)
}

fmt.Printf("Patient %s has %d changes\n", history.PrimaryKeyStr, history.ChangeCount)
fmt.Printf("Changed by: %v\n", history.ChangedByList)
fmt.Printf("Operations: %v\n", history.Operations)
```

### Getting Statistics

```go
// Get audit statistics for a date range
stats, err := auditRepo.GetStats(context.Background(), "appointment", "Appointment",
	time.Now().AddDate(0, -1, 0), time.Now())
if err != nil {
	log.Fatal(err)
}

fmt.Printf("Total changes: %d\n", stats.TotalRecords)
fmt.Printf("Unique users: %d\n", stats.UniqueUsers)
fmt.Printf("Operations: %v\n", stats.OperationCounts)
```

## Data Models

### DataChangeLog

Represents a single audit log entry:

```go
type DataChangeLog struct {
	ID              string                 // Unique identifier
	Domain          string                 // Service domain
	Entity          string                 // Entity type
	Operation       string                 // CREATE, UPDATE, DELETE
	PrimaryKey      map[string]interface{} // Primary key as map
	PrimaryKeyStr   string                 // Primary key as string
	BeforeData      map[string]interface{} // Before snapshot (optional)
	AfterData       map[string]interface{} // After snapshot
	Changes         []FieldDiff            // Field-level changes
	ChangedBy       string                 // User ID
	ChangedByEmail  string                 // User email
	ChangeTimestamp time.Time              // When change occurred
	RequestID       string                 // Request trace ID
	IPAddress       string                 // Client IP
	UserAgent       string                 // Client user agent
	Metadata        map[string]interface{} // Custom metadata
}
```

### FieldDiff

Represents a change in a single field:

```go
type FieldDiff struct {
	FieldName   string      // Field name
	FieldType   string      // Field type (string, number, boolean, etc.)
	OldValue    interface{} // Value before change
	NewValue    interface{} // Value after change
	Sanitized   bool        // Was value sanitized?
	Transformer string      // Transformer applied
}
```

## Elasticsearch Index Template

The middleware creates indices with the pattern: `audit-log-{domain}-{yyyy.MM}`

Example: `audit-log-appointment-2024.01`

The index mapping includes:

- Keyword fields for efficient filtering (domain, entity, operation, primary_key_str)
- Nested fields for changes array
- Date fields for timestamps
- Text fields for searchable data

## Sensitive Field Handling

The middleware automatically sanitizes these field types:

- **Passwords & Secrets**: password, pwd, secret, token, api_key
- **Authentication**: access_token, refresh_token, authorization
- **Personal IDs**: ssn, nin, nric, passport_number, driver_license
- **Medical**: blood_type, diagnosis, prescription, medication
- **Contact Info**: phone, mobile, email, address
- **Financial**: credit_card, bank_account, insurance_id
- **Healthcare**: bpjs_number, insurance_number

Redaction strategies:

- **Hash**: SHA256 hash with visible prefix
- **Mask**: Show first/last few characters only
- **Redaction**: Full asterisks

Configure via `global.sensitive_fields` in config.

## Performance Tuning

### Worker Pool Configuration

Adjust based on your throughput:

```yaml
elasticsearch:
  num_workers: 8      # More workers for high throughput
  bulk_size: 500      # Larger batches = better efficiency
  flush_interval: 5s  # Longer interval to accumulate more records
```

### Index Rotation

Change index pattern for faster queries:

```yaml
elasticsearch:
  index_pattern: "{prefix}-{domain}-{yyyy.MM.dd}"  # Daily indices
```

### Excluded Fields

Reduce storage by excluding unnecessary fields:

```yaml
global:
  excluded_fields:
    - "internal_status"
    - "debug_flag"
    - "temporary_data"
```

## Testing

Use the mock repository for testing:

```go
func TestAuditLogging(t *testing.T) {
	repo := datachangelog.NewMockElasticsearchRepository()
	
	// Create audit log
	log := &datachangelog.DataChangeLog{
		ID:        uuid.New().String(),
		Domain:    "appointment",
		Entity:    "Appointment",
		Operation: "CREATE",
		// ... populate other fields
	}
	
	err := repo.Save(context.Background(), log)
	require.NoError(t, err)
	
	// Query logs
	result, err := repo.Query(context.Background(), &datachangelog.ChangeLogQuery{
		Domain: "appointment",
		Entity: "Appointment",
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), result.Total)
}
```

## Production Deployment

### Elasticsearch Setup

1. Install Elasticsearch (7.10+)
2. Configure authentication
3. Set up index lifecycle policies for retention
4. Configure backups

### Index Templates

Apply the index template to Elasticsearch:

```bash
curl -X PUT "http://localhost:9200/_index_template/audit-log" \
  -H "Content-Type: application/json" \
  -d @audit-log-template.json
```

### Monitoring

Monitor these metrics:

- Queue depth (pending logs)
- Indexing latency
- Failed logs count
- Elasticsearch cluster health

### Retention

Configure Elasticsearch Index Lifecycle Management (ILM) to:

- Roll over indices monthly
- Archive old indices
- Delete after retention period (e.g., 1 year)

## Troubleshooting

### Logs Not Being Saved

1. Check Elasticsearch connection:
   ```go
   err := repo.Health(context.Background())
   ```

2. Verify configuration is loaded correctly

3. Check worker pool status (logs may be queued)

### High Memory Usage

- Reduce `bulk_size`
- Reduce `num_workers`
- Increase `flush_interval`

### Slow Queries

- Create indices with appropriate sharding
- Add filters on frequently queried fields
- Use Elasticsearch aggregations for statistics

## Security Considerations

1. **Authentication**: Use strong credentials for Elasticsearch
2. **SSL/TLS**: Enable in production
3. **Network**: Restrict access to Elasticsearch
4. **Audit Log Access**: Implement role-based access to audit logs
5. **Data Retention**: Comply with data protection regulations
6. **Sanitization**: Verify sensitive fields are properly redacted

## API Reference

### Repository Interface

```go
type Repository interface {
	Save(ctx context.Context, log *DataChangeLog) error
	SaveBatch(ctx context.Context, logs []DataChangeLog) error
	Query(ctx context.Context, query *ChangeLogQuery) (*ChangeLogQueryResult, error)
	GetByPrimaryKey(ctx context.Context, domain, entity, primaryKey string, limit, offset int) (*ChangeLogQueryResult, error)
	GetEntityHistory(ctx context.Context, domain, entity, primaryKey string) (*EntityChangeHistory, error)
	DeleteOlderThan(ctx context.Context, domain, entity string, date time.Time) error
	GetStats(ctx context.Context, domain, entity string, startDate, endDate time.Time) (*AuditStats, error)
	Close() error
	Health(ctx context.Context) error
}
```

## Next Steps

1. Implement real Elasticsearch repository (see `elasticsearch_impl.go` template)
2. Add custom user and IP extractors for your authentication system
3. Configure field transformers for domain-specific logic
4. Set up monitoring and alerting
5. Plan retention policy and archive strategy

## Support

For issues or questions:
1. Check the configuration file format
2. Verify Elasticsearch connectivity
3. Review field mapping in index template
4. Check application logs for error messages