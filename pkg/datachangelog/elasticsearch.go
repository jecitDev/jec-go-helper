package datachangelog

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
	"github.com/google/uuid"
)

// ElasticsearchRepository is the production implementation of the Repository interface
// using Elasticsearch as the backend storage for audit logs
type ElasticsearchRepository struct {
	client     *elasticsearch.Client
	config     *ElasticsearchConfig
	indexName  string
	bulkWriter *BulkIndexWriter
	mu         sync.RWMutex
}

// BulkIndexWriter handles asynchronous bulk indexing of audit logs
type BulkIndexWriter struct {
	repo          *ElasticsearchRepository
	queue         chan *DataChangeLog
	batchSize     int
	flushInterval time.Duration
	stopChan      chan struct{}
	wg            sync.WaitGroup
	mutex         sync.Mutex
	status        BatchWriterStatus
}

// NewElasticsearchRepository creates and initializes a new Elasticsearch repository
// for storing and retrieving audit logs.
//
// Parameters:
//   - config: ElasticsearchConfig containing connection details and settings
//   - maxRetries: maximum number of retry attempts for failed operations
//
// Returns:
//   - *ElasticsearchRepository: configured repository instance
//   - error: if connection fails or configuration is invalid
//
// Example:
//
//	config := &ElasticsearchConfig{
//		Addresses: []string{"https://localhost:9200"},
//		Username:  "elastic",
//		Password:  "password",
//		IndexPrefix: "audit-log",
//	}
//	repo, err := NewElasticsearchRepository(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer repo.Close()
func NewElasticsearchRepository(config *ElasticsearchConfig) (*ElasticsearchRepository, error) {
	if config == nil {
		return nil, fmt.Errorf("elasticsearch config cannot be nil")
	}

	if len(config.Addresses) == 0 {
		return nil, fmt.Errorf("elasticsearch addresses must be specified")
	}

	// Create Elasticsearch client configuration
	escfg := elasticsearch.Config{
		Addresses: config.Addresses,
		Username:  config.Username,
		Password:  config.Password,
		// APIKey:     config.APIKey,
		MaxRetries: config.MaxRetries,
	}

	// Configure TLS if needed
	if config.InsecureSkipVerify || config.CACert != "" {
		// This will be handled via transport configuration in production
		// For now, we set the basic flag
	}

	if config.InsecureSkipVerify {
		escfg.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	// Create client
	client, err := elasticsearch.NewClient(escfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create elasticsearch client: %w", err)
	}

	// Test connection
	res, err := client.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to elasticsearch: %w", err)
	}

	if res.StatusCode >= 400 {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return nil, fmt.Errorf("elasticsearch returned error: %s", string(body))
	}
	res.Body.Close()

	repo := &ElasticsearchRepository{
		client:    client,
		config:    config,
		indexName: config.IndexPrefix,
	}

	// Start bulk writer for asynchronous writes
	if config.NumWorkers > 0 && config.BulkSize > 0 {
		repo.bulkWriter = NewBulkIndexWriter(repo, config.BulkSize, config.FlushInterval)
		repo.bulkWriter.Start(config.NumWorkers)
	}

	return repo, nil
}

// Save persists a single data change log entry to Elasticsearch
func (r *ElasticsearchRepository) Save(ctx context.Context, log *DataChangeLog) error {
	if log == nil {
		return fmt.Errorf("log cannot be nil")
	}

	// Generate ID if not provided
	if log.ID == "" {
		log.ID = uuid.New().String()
	}

	// Use bulk writer if available for async writes
	// if r.bulkWriter != nil && r.bulkWriter.IsRunning() {
	// 	return r.bulkWriter.Write(log)
	// }

	// Fallback to synchronous write
	return r.saveDirect(ctx, log)
}

// saveDirect synchronously saves a log entry
func (r *ElasticsearchRepository) saveDirect(ctx context.Context, log *DataChangeLog) error {
	// Serialize log to JSON
	logBytes, err := json.Marshal(log)
	if err != nil {
		return fmt.Errorf("failed to marshal log to JSON: %w", err)
	}

	// Generate index name based on timestamp
	indexName := r.generateIndexName(log.Domain, log.ChangeTimestamp)

	// Index the document
	req := esapi.IndexRequest{
		Index:      indexName,
		DocumentID: log.ID,
		Body:       bytes.NewReader(logBytes),
		Refresh:    "false", // Use bulk refresh for better performance
	}

	res, err := req.Do(ctx, r.client)
	if err != nil {
		return fmt.Errorf("failed to index document: %w", err)
	}
	if res.StatusCode >= 400 {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return fmt.Errorf("elasticsearch returned error: %s", string(body))
	}
	res.Body.Close()

	return nil
}

// SaveBatch persists multiple data change log entries in a single batch operation
func (r *ElasticsearchRepository) SaveBatch(ctx context.Context, logs []DataChangeLog) error {
	if logs == nil || len(logs) == 0 {
		return nil
	}

	// If bulk writer is available, queue all logs
	if r.bulkWriter != nil && r.bulkWriter.IsRunning() {
		for i := range logs {
			if logs[i].ID == "" {
				logs[i].ID = uuid.New().String()
			}
			if err := r.bulkWriter.Write(&logs[i]); err != nil {
				return err
			}
		}
		return nil
	}

	// Fallback to synchronous bulk write
	return r.saveBatchDirect(ctx, logs)
}

// saveBatchDirect synchronously saves multiple logs using Elasticsearch bulk API
func (r *ElasticsearchRepository) saveBatchDirect(ctx context.Context, logs []DataChangeLog) error {
	var buf bytes.Buffer

	for i := range logs {
		if logs[i].ID == "" {
			logs[i].ID = uuid.New().String()
		}

		indexName := r.generateIndexName(logs[i].Domain, logs[i].ChangeTimestamp)

		// Write bulk action metadata
		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": indexName,
				"_id":    logs[i].ID,
			},
		}
		metaBytes, _ := json.Marshal(meta)
		buf.Write(metaBytes)
		buf.WriteString("\n")

		// Write document
		docBytes, _ := json.Marshal(logs[i])
		buf.Write(docBytes)
		buf.WriteString("\n")
	}

	// Execute bulk request
	req := esapi.BulkRequest{
		Body: &buf,
	}

	res, err := req.Do(ctx, r.client)
	if err != nil {
		return fmt.Errorf("failed to execute bulk request: %w", err)
	}
	if res.StatusCode >= 400 {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return fmt.Errorf("elasticsearch bulk request returned error: %s", string(body))
	}
	res.Body.Close()

	// Check bulk response for individual errors
	var bulkRes map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&bulkRes); err != nil {
		return fmt.Errorf("failed to parse bulk response: %w", err)
	}

	if hasErrors, ok := bulkRes["errors"].(bool); ok && hasErrors {
		return fmt.Errorf("bulk request had errors, check Elasticsearch logs for details")
	}

	return nil
}

// Query retrieves audit logs based on query parameters
func (r *ElasticsearchRepository) Query(ctx context.Context, query *ChangeLogQuery) (*ChangeLogQueryResult, error) {
	if query == nil {
		query = &ChangeLogQuery{}
	}

	// Build Elasticsearch query
	esQuery := r.buildQuery(query)

	// Create search request
	searchBody, err := json.Marshal(esQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	// Search across all audit indices
	searchPattern := fmt.Sprintf("%s-*", r.config.IndexPrefix)
	if query.Domain != "" {
		searchPattern = fmt.Sprintf("%s-%s-*", r.config.IndexPrefix, query.Domain)
	}

	req := esapi.SearchRequest{
		Index: []string{searchPattern},
		Body:  bytes.NewReader(searchBody),
		Size:  &query.Limit,
		From:  &query.Offset,
	}

	res, err := req.Do(ctx, r.client)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}
	if res.StatusCode >= 400 {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return nil, fmt.Errorf("elasticsearch returned error: %s", string(body))
	}
	res.Body.Close()

	// Parse response
	var esRes map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&esRes); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	// Extract results
	return r.parseSearchResults(esRes, query), nil
}

// GetByPrimaryKey retrieves all changes for a specific entity by primary key
func (r *ElasticsearchRepository) GetByPrimaryKey(ctx context.Context, domain, entity, primaryKey string, limit, offset int) (*ChangeLogQueryResult, error) {
	query := &ChangeLogQuery{
		Domain:        domain,
		Entity:        entity,
		PrimaryKeyStr: primaryKey,
		Limit:         limit,
		Offset:        offset,
	}

	return r.Query(ctx, query)
}

// GetEntityHistory retrieves the complete change history for an entity
func (r *ElasticsearchRepository) GetEntityHistory(ctx context.Context, domain, entity, primaryKey string) (*EntityChangeHistory, error) {
	// Query all changes for the entity
	query := &ChangeLogQuery{
		Domain:        domain,
		Entity:        entity,
		PrimaryKeyStr: primaryKey,
		Limit:         10000, // Get all changes
	}

	result, err := r.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	// Build history from results
	history := &EntityChangeHistory{
		Domain:        domain,
		Entity:        entity,
		PrimaryKeyStr: primaryKey,
		Changes:       result.Records,
		ChangedByList: []string{},
		Operations:    make(map[string]int64),
	}

	userSet := make(map[string]bool)

	for _, log := range result.Records {
		history.ChangeCount++

		if log.ChangedBy != "" {
			userSet[log.ChangedBy] = true
		}

		history.Operations[log.Operation]++

		if history.FirstChange.IsZero() || log.ChangeTimestamp.Before(history.FirstChange) {
			history.FirstChange = log.ChangeTimestamp
		}
		if log.ChangeTimestamp.After(history.LastChange) {
			history.LastChange = log.ChangeTimestamp
		}
	}

	for user := range userSet {
		history.ChangedByList = append(history.ChangedByList, user)
	}

	return history, nil
}

// DeleteOlderThan deletes audit logs older than the specified date
func (r *ElasticsearchRepository) DeleteOlderThan(ctx context.Context, domain, entity string, date time.Time) error {
	// Build delete by query request
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"range": map[string]interface{}{
							"change_timestamp": map[string]interface{}{
								"lt": date.UTC(),
							},
						},
					},
				},
			},
		},
	}

	if domain != "" {
		query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = append(
			query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]map[string]interface{}),
			map[string]interface{}{
				"term": map[string]interface{}{
					"domain.keyword": domain,
				},
			},
		)
	}

	if entity != "" {
		query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = append(
			query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]map[string]interface{}),
			map[string]interface{}{
				"term": map[string]interface{}{
					"entity.keyword": entity,
				},
			},
		)
	}

	queryBytes, _ := json.Marshal(query)

	searchPattern := fmt.Sprintf("%s-*", r.config.IndexPrefix)
	if domain != "" {
		searchPattern = fmt.Sprintf("%s-%s-*", r.config.IndexPrefix, domain)
	}

	req := esapi.DeleteByQueryRequest{
		Index: []string{searchPattern},
		Body:  bytes.NewReader(queryBytes),
	}

	res, err := req.Do(ctx, r.client)
	if err != nil {
		return fmt.Errorf("failed to execute delete by query: %w", err)
	}
	if res.StatusCode >= 400 {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return fmt.Errorf("elasticsearch returned error: %s", string(body))
	}
	res.Body.Close()

	return nil
}

// GetStats returns statistics about audit logs
func (r *ElasticsearchRepository) GetStats(ctx context.Context, domain, entity string, startDate, endDate time.Time) (*AuditStats, error) {
	stats := &AuditStats{
		Domain:          domain,
		Entity:          entity,
		OperationCounts: make(map[string]int64),
		DateRange: DateRange{
			Start: startDate,
			End:   endDate,
		},
	}

	// Build aggregation query
	aggs := map[string]interface{}{
		"operations": map[string]interface{}{
			"terms": map[string]interface{}{
				"field": "operation.keyword",
				"size":  100,
			},
		},
		"users": map[string]interface{}{
			"cardinality": map[string]interface{}{
				"field": "changed_by.keyword",
			},
		},
		"entities": map[string]interface{}{
			"cardinality": map[string]interface{}{
				"field": "primary_key_str.keyword",
			},
		},
	}

	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"range": map[string]interface{}{
							"change_timestamp": map[string]interface{}{
								"gte": startDate.UTC(),
								"lte": endDate.UTC(),
							},
						},
					},
				},
			},
		},
		"aggs": aggs,
		"size": 0,
	}

	if domain != "" {
		query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = append(
			query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]map[string]interface{}),
			map[string]interface{}{
				"term": map[string]interface{}{
					"domain.keyword": domain,
				},
			},
		)
	}

	if entity != "" {
		query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = append(
			query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]map[string]interface{}),
			map[string]interface{}{
				"term": map[string]interface{}{
					"entity.keyword": entity,
				},
			},
		)
	}

	queryBytes, _ := json.Marshal(query)

	searchPattern := fmt.Sprintf("%s-*", r.config.IndexPrefix)
	if domain != "" {
		searchPattern = fmt.Sprintf("%s-%s-*", r.config.IndexPrefix, domain)
	}

	req := esapi.SearchRequest{
		Index: []string{searchPattern},
		Body:  bytes.NewReader(queryBytes),
	}

	res, err := req.Do(ctx, r.client)
	if err != nil {
		return nil, fmt.Errorf("failed to execute stats query: %w", err)
	}
	if res.StatusCode >= 400 {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return nil, fmt.Errorf("elasticsearch returned error: %s", string(body))
	}
	res.Body.Close()

	// Parse aggregation results
	var esRes map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&esRes); err != nil {
		return nil, fmt.Errorf("failed to parse stats response: %w", err)
	}

	r.parseAggregations(stats, esRes)

	// Get total count
	if hits, ok := esRes["hits"].(map[string]interface{}); ok {
		if total, ok := hits["total"].(map[string]interface{}); ok {
			if count, ok := total["value"].(float64); ok {
				stats.TotalRecords = int64(count)
			}
		}
	}

	return stats, nil
}

// Close closes the Elasticsearch client and stops the bulk writer
func (r *ElasticsearchRepository) Close() error {
	if r.bulkWriter != nil {
		if err := r.bulkWriter.Close(); err != nil {
			return err
		}
	}

	// Elasticsearch client doesn't require explicit close
	// The underlying HTTP transport will be cleaned up by GC

	return nil
}

// Health checks if the Elasticsearch repository is healthy and accessible
func (r *ElasticsearchRepository) Health(ctx context.Context) error {
	res, err := r.client.Info()
	if err != nil {
		return fmt.Errorf("elasticsearch health check failed: %w", err)
	}

	if res.StatusCode >= 400 {
		return fmt.Errorf("elasticsearch health check error, status code: %d", res.StatusCode)
	}
	res.Body.Close()

	return nil
}

// Helper methods

// generateIndexName generates the Elasticsearch index name based on domain and timestamp
func (r *ElasticsearchRepository) generateIndexName(domain string, timestamp time.Time) string {
	// Format: audit-log-{domain}-yyyy.MM
	return fmt.Sprintf("%s-%s-%04d.%02d", r.indexName, domain, timestamp.Year(), timestamp.Month())
}

// buildQuery constructs an Elasticsearch query from ChangeLogQuery parameters
func (r *ElasticsearchRepository) buildQuery(q *ChangeLogQuery) map[string]interface{} {
	must := []map[string]interface{}{}

	if q.Domain != "" {
		must = append(must, map[string]interface{}{
			"term": map[string]interface{}{
				"domain.keyword": q.Domain,
			},
		})
	}

	if q.Entity != "" {
		must = append(must, map[string]interface{}{
			"term": map[string]interface{}{
				"entity.keyword": q.Entity,
			},
		})
	}

	if q.PrimaryKeyStr != "" {
		must = append(must, map[string]interface{}{
			"term": map[string]interface{}{
				"primary_key_str.keyword": q.PrimaryKeyStr,
			},
		})
	}

	if q.Operation != "" {
		must = append(must, map[string]interface{}{
			"term": map[string]interface{}{
				"operation.keyword": q.Operation,
			},
		})
	}

	if q.ChangedBy != "" {
		must = append(must, map[string]interface{}{
			"term": map[string]interface{}{
				"changed_by.keyword": q.ChangedBy,
			},
		})
	}

	if !q.StartDate.IsZero() || !q.EndDate.IsZero() {
		rangeQuery := map[string]interface{}{}
		if !q.StartDate.IsZero() {
			rangeQuery["gte"] = q.StartDate.UTC()
		}
		if !q.EndDate.IsZero() {
			rangeQuery["lte"] = q.EndDate.UTC()
		}
		must = append(must, map[string]interface{}{
			"range": map[string]interface{}{
				"change_timestamp": rangeQuery,
			},
		})
	}

	if len(must) == 0 {
		// Match all if no filters
		return map[string]interface{}{
			"query": map[string]interface{}{
				"match_all": map[string]interface{}{},
			},
		}
	}

	return map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": must,
			},
		},
	}
}

// parseSearchResults converts Elasticsearch search response to ChangeLogQueryResult
func (r *ElasticsearchRepository) parseSearchResults(res map[string]interface{}, q *ChangeLogQuery) *ChangeLogQueryResult {
	result := &ChangeLogQueryResult{
		Limit:   q.Limit,
		Offset:  q.Offset,
		Records: []DataChangeLog{},
	}

	hits, ok := res["hits"].(map[string]interface{})
	if !ok {
		return result
	}

	// Get total count
	if total, ok := hits["total"].(map[string]interface{}); ok {
		if count, ok := total["value"].(float64); ok {
			result.Total = int64(count)
		}
	}

	// Extract records
	if hitList, ok := hits["hits"].([]interface{}); ok {
		for _, hit := range hitList {
			if hitMap, ok := hit.(map[string]interface{}); ok {
				if source, ok := hitMap["_source"].(map[string]interface{}); ok {
					var log DataChangeLog
					sourceBytes, _ := json.Marshal(source)
					json.Unmarshal(sourceBytes, &log)
					result.Records = append(result.Records, log)
				}
			}
		}
	}

	return result
}

// parseAggregations extracts aggregation results from Elasticsearch response
func (r *ElasticsearchRepository) parseAggregations(stats *AuditStats, res map[string]interface{}) {
	aggs, ok := res["aggregations"].(map[string]interface{})
	if !ok {
		return
	}

	// Parse operations
	if opsAgg, ok := aggs["operations"].(map[string]interface{}); ok {
		if buckets, ok := opsAgg["buckets"].([]interface{}); ok {
			for _, bucket := range buckets {
				if b, ok := bucket.(map[string]interface{}); ok {
					if key, ok := b["key"].(string); ok {
						if count, ok := b["doc_count"].(float64); ok {
							stats.OperationCounts[key] = int64(count)
						}
					}
				}
			}
		}
	}

	// Parse users cardinality
	if usersAgg, ok := aggs["users"].(map[string]interface{}); ok {
		if count, ok := usersAgg["value"].(float64); ok {
			stats.UniqueUsers = int64(count)
		}
	}

	// Parse entities cardinality
	if entitiesAgg, ok := aggs["entities"].(map[string]interface{}); ok {
		if count, ok := entitiesAgg["value"].(float64); ok {
			stats.UniqueEntities = int64(count)
		}
	}
}

// NewBulkIndexWriter creates a new bulk index writer
func NewBulkIndexWriter(repo *ElasticsearchRepository, batchSize int, flushInterval time.Duration) *BulkIndexWriter {
	return &BulkIndexWriter{
		repo:          repo,
		queue:         make(chan *DataChangeLog, batchSize*2),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		stopChan:      make(chan struct{}),
		status: BatchWriterStatus{
			IsRunning: false,
		},
	}
}

// Start starts the bulk writer workers
func (b *BulkIndexWriter) Start(numWorkers int) {
	b.mutex.Lock()
	b.status.IsRunning = true
	b.mutex.Unlock()

	for i := 0; i < numWorkers; i++ {
		b.wg.Add(1)
		go b.worker()
	}
}

// worker processes logs from the queue and performs bulk writes
func (b *BulkIndexWriter) worker() {
	defer b.wg.Done()

	batch := make([]DataChangeLog, 0, b.batchSize)
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopChan:
			// Flush remaining logs
			if len(batch) > 0 {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				b.repo.saveBatchDirect(ctx, batch)
				cancel()
			}
			return

		case log := <-b.queue:
			if log != nil {
				batch = append(batch, *log)
				b.updateStatus(func() {
					b.status.QueueSize = len(b.queue)
				})

				if len(batch) >= b.batchSize {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					b.repo.saveBatchDirect(ctx, batch)
					cancel()

					b.updateStatus(func() {
						b.status.ProcessedCount += int64(len(batch))
						b.status.LastFlushTime = time.Now()
					})

					batch = make([]DataChangeLog, 0, b.batchSize)
				}
			}

		case <-ticker.C:
			if len(batch) > 0 {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				b.repo.saveBatchDirect(ctx, batch)
				cancel()

				b.updateStatus(func() {
					b.status.ProcessedCount += int64(len(batch))
					b.status.LastFlushTime = time.Now()
				})

				batch = make([]DataChangeLog, 0, b.batchSize)
			}
		}
	}
}

// Write queues a log entry for batch writing
func (b *BulkIndexWriter) Write(log *DataChangeLog) error {
	select {
	case b.queue <- log:
		return nil
	case <-b.stopChan:
		return fmt.Errorf("bulk writer is stopped")
	default:
		return fmt.Errorf("bulk writer queue is full")
	}
}

// Flush writes all queued entries immediately
func (b *BulkIndexWriter) Flush(ctx context.Context) error {
	// Signal workers to flush
	close(b.stopChan)
	b.wg.Wait()
	return nil
}

// Close gracefully closes the writer
func (b *BulkIndexWriter) Close() error {
	b.mutex.Lock()
	b.status.IsRunning = false
	b.mutex.Unlock()

	return b.Flush(context.Background())
}

// Status returns the current status of the writer
func (b *BulkIndexWriter) Status() BatchWriterStatus {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.status
}

// IsRunning returns whether the bulk writer is running
func (b *BulkIndexWriter) IsRunning() bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.status.IsRunning
}

// updateStatus safely updates the status
func (b *BulkIndexWriter) updateStatus(fn func()) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	fn()
}
