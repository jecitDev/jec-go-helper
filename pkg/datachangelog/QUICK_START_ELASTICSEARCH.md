# Quick Start: NewElasticsearchRepository

## TL;DR - Get Started in 5 Minutes

### 1. Import the package
```go
import "jecis.admission/infra/datachangelog"
```

### 2. Create configuration
```go
config := &datachangelog.ElasticsearchConfig{
    Enabled:        true,
    Addresses:      []string{"https://elasticsearch:9200"},
    Username:       "elastic",
    Password:       "your-password",
    IndexPrefix:    "audit-log",
    NumWorkers:     4,
    BulkSize:       100,
    FlushInterval:  2 * time.Second,
    RequestTimeout: 10 * time.Second,
}
```

### 3. Create repository
```go
repo, err := datachangelog.NewElasticsearchRepository(config)
if err != nil {
    log.Fatalf("Failed to create repository: %v", err)
}
defer repo.Close()
```

### 4. Save an audit log
```go
log := &datachangelog.DataChangeLog{
    Domain:          "appointment",
    Entity:          "Appointment",
    Operation:       "CREATE",
    PrimaryKeyStr:   "APT-001:BU-001",
    ChangedBy:       "user123",
    ChangeTimestamp: time.Now(),
}

if err := repo.Save(context.Background(), log); err != nil {
    log.Printf("Error: %v", err)
}
```

### 5. Query audit logs
```go
result, err := repo.Query(context.Background(), &datachangelog.ChangeLogQuery{
    Domain: "appointment",
    Entity: "Appointment",
    Limit:  10,
})

for _, record := range result.Records {
    fmt.Printf("Log: %s by %s\n", record.ID, record.ChangedBy)
}
```

---

## Common Configurations

### Development (Local Elasticsearch)
```go
config := &datachangelog.ElasticsearchConfig{
    Enabled:            true,
    Addresses:          []string{"http://localhost:9200"},
    Username:           "elastic",
    Password:           "changeme",
    IndexPrefix:        "audit-log",
    InsecureSkipVerify: true,
    NumWorkers:         2,
    BulkSize:           50,
    FlushInterval:      1 * time.Second,
}
```

### Production (Elastic Cloud)
```go
config := &datachangelog.ElasticsearchConfig{
    Enabled:        true,
    Addresses:      []string{
        "https://abc123.us-east-1.aws.cloud.es.io:9243",
    },
    APIKey:         os.Getenv("ELASTICSEARCH_API_KEY"),
    IndexPrefix:    "audit-log",
    NumWorkers:     8,
    BulkSize:       500,
    MaxRetries:     5,
    FlushInterval:  5 * time.Second,
}
```

### High-Volume Production
```go
config := &datachangelog.ElasticsearchConfig{
    Enabled:        true,
    Addresses:      []string{
        "https://es1:9200",
        "https://es2:9200",
        "https://es3:9200",
    },
    Username:       "audit_service",
    Password:       os.Getenv("ELASTICSEARCH_PASSWORD"),
    IndexPrefix:    "audit-log",
    IndexPattern:   "{prefix}-{domain}-{yyyy.MM.dd}",
    NumWorkers:     16,
    BulkSize:       1000,
    MaxRetries:     3,
    FlushInterval:  10 * time.Second,
    RequestTimeout: 60 * time.Second,
}
```

---

## Common Operations

### Save Single Log
```go
log := &datachangelog.DataChangeLog{
    Domain:          "appointment",
    Entity:          "Appointment",
    Operation:       "UPDATE",
    PrimaryKeyStr:   "APT-001:BU-001",
    BeforeData:      map[string]interface{}{"status": "pending"},
    AfterData:       map[string]interface{}{"status": "confirmed"},
    ChangedBy:       "doctor@hospital.com",
    ChangeTimestamp: time.Now(),
}
repo.Save(ctx, log)
```

### Save Batch
```go
logs := []datachangelog.DataChangeLog{
    {/* log 1 */},
    {/* log 2 */},
    {/* log 3 */},
}
repo.SaveBatch(ctx, logs)
```

### Query by Domain & Entity
```go
result, _ := repo.Query(ctx, &datachangelog.ChangeLogQuery{
    Domain: "appointment",
    Entity: "Appointment",
    Limit:  50,
    Offset: 0,
})
```

### Query by Operation
```go
result, _ := repo.Query(ctx, &datachangelog.ChangeLogQuery{
    Domain:    "appointment",
    Entity:    "Appointment",
    Operation: "UPDATE",
})
```

### Query by Date Range
```go
result, _ := repo.Query(ctx, &datachangelog.ChangeLogQuery{
    Domain:    "appointment",
    Entity:    "Appointment",
    StartDate: time.Now().AddDate(0, 0, -7),
    EndDate:   time.Now(),
})
```

### Get Entity History
```go
history, _ := repo.GetEntityHistory(ctx, "appointment", "Appointment", "APT-001:BU-001")
fmt.Printf("Total changes: %d\n", history.ChangeCount)
fmt.Printf("Changed by: %v\n", history.ChangedByList)
```

### Get Statistics
```go
stats, _ := repo.GetStats(ctx, "appointment", "Appointment",
    time.Now().AddDate(0, -1, 0),
    time.Now(),
)
fmt.Printf("Total records: %d\n", stats.TotalRecords)
fmt.Printf("Unique users: %d\n", stats.UniqueUsers)
```

### Delete Old Logs
```go
cutoff := time.Now().AddDate(0, -3, 0) // 90 days ago
repo.DeleteOlderThan(ctx, "appointment", "Appointment", cutoff)
```

### Health Check
```go
if err := repo.Health(ctx); err != nil {
    log.Printf("Repository unhealthy: %v", err)
}
```

---

## Error Handling

### Connection Errors
```go
repo, err := datachangelog.NewElasticsearchRepository(config)
if err != nil {
    if strings.Contains(err.Error(), "health check failed") {
        // Elasticsearch not accessible
        // Fallback to mock repository
        repo = datachangelog.NewMockElasticsearchRepository()
    }
}
```

### Query Errors
```go
result, err := repo.Query(ctx, query)
if err != nil {
    if strings.Contains(err.Error(), "timeout") {
        // Query took too long - try with smaller limit
        query.Limit = 10
        result, _ = repo.Query(ctx, query)
    }
}
```

### Bulk Write Errors
```go
err := repo.SaveBatch(ctx, logs)
if err != nil {
    log.Printf("Bulk write failed: %v", err)
    // Can retry manually or queue for later
}
```

---

## Configuration from YAML

```yaml
elasticsearch:
  enabled: true
  addresses:
    - https://elasticsearch:9200
  username: elastic
  password: ${ELASTICSEARCH_PASSWORD}
  index_prefix: audit-log
  index_pattern: "{prefix}-{domain}-{yyyy.MM}"
  num_workers: 4
  bulk_size: 100
  max_retries: 3
  retry_delay: 500ms
  flush_interval: 2s
  request_timeout: 10s
```

Load and use:
```go
configBytes, _ := ioutil.ReadFile("config/datachangelog_config.yaml")
cfg, _ := datachangelog.LoadConfig(configBytes)

repo, err := datachangelog.NewElasticsearchRepository(cfg.Elasticsearch)
```

---

## Integration with gRPC Handlers

### In your handler
```go
func (h *AppointmentHandler) CreateAppointment(ctx context.Context, req *pb.CreateAppointmentRequest) (*pb.CreateAppointmentResponse, error) {
    // Create appointment logic
    appt := &Appointment{/* ... */}
    
    // Log audit
    auditLog := &datachangelog.DataChangeLog{
        Domain:          "appointment",
        Entity:          "Appointment",
        Operation:       "CREATE",
        PrimaryKeyStr:   appt.GetPrimaryKey(),
        AfterData:       appt.ToMap(),
        ChangedBy:       getUserID(ctx),
        ChangeTimestamp: time.Now(),
    }
    h.auditRepo.Save(ctx, auditLog)
    
    return &pb.CreateAppointmentResponse{
        Appointment: appt.ToProto(),
    }, nil
}
```

---

## Performance Tips

1. **Batch Size**: Use 500-1000 for high volume, 50-100 for low latency
2. **Workers**: Increase to 8-16 for concurrent writes
3. **Flush Interval**: Shorter (1s) for real-time, longer (10s) for throughput
4. **Connections**: Use 3+ Elasticsearch nodes for HA
5. **Indices**: Use daily rotation for very large volumes

---

## Monitoring

### Check queue size
```go
// If using bulk writer
// Monitor queue depth to detect backlog
```

### Health monitoring
```go
ticker := time.NewTicker(30 * time.Second)
for range ticker.C {
    if err := repo.Health(ctx); err != nil {
        log.Printf("Repository unhealthy: %v", err)
    }
}
```

### Stats reporting
```go
stats, _ := repo.GetStats(ctx, "appointment", "Appointment", start, end)
log.Printf("Write rate: %.0f/sec", float64(stats.TotalRecords)/duration.Seconds())
```

---

## Troubleshooting

### Connection fails
```
✓ Check Elasticsearch is running
✓ Verify addresses and port
✓ Check username/password
✓ Verify TLS certificates if HTTPS
✓ Check firewall rules
```

### Slow queries
```
✓ Add indices to filter on
✓ Use smaller date ranges
✓ Reduce limit/offset
✓ Check Elasticsearch cluster health
```

### High memory usage
```
✓ Reduce BulkSize (lower buffering)
✓ Reduce NumWorkers (fewer goroutines)
✓ Increase FlushInterval (flush less often)
```

### Index issues
```
✓ Check index prefix configuration
✓ Verify index lifecycle policies
✓ Check disk space available
✓ Monitor shard allocation
```

---

## See Also

- [Elasticsearch Repository Documentation](ELASTICSEARCH_REPOSITORY.md)
- [Data Change Log Models](models.go)
- [Repository Interface](repository.go)
- [Configuration Guide](config.go)
