package datachangelog

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MockElasticsearchRepository is a mock implementation of the Repository interface
// for testing and development without requiring a real Elasticsearch instance
type MockElasticsearchRepository struct {
	mu    sync.RWMutex
	logs  map[string]*DataChangeLog
	index int64
}

// NewMockElasticsearchRepository creates a new mock repository
func NewMockElasticsearchRepository() *MockElasticsearchRepository {
	return &MockElasticsearchRepository{
		logs: make(map[string]*DataChangeLog),
	}
}

// Save saves a single audit log entry
func (m *MockElasticsearchRepository) Save(ctx context.Context, log *DataChangeLog) error {
	if log == nil {
		return fmt.Errorf("log cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if log.ID == "" {
		return fmt.Errorf("log ID cannot be empty")
	}

	m.logs[log.ID] = log
	m.index++

	return nil
}

// SaveBatch saves multiple audit log entries in a single operation
func (m *MockElasticsearchRepository) SaveBatch(ctx context.Context, logs []DataChangeLog) error {
	if logs == nil || len(logs) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range logs {
		if logs[i].ID == "" {
			logs[i].ID = fmt.Sprintf("batch-%d-%d", m.index, i)
		}
		m.logs[logs[i].ID] = &logs[i]
	}

	m.index += int64(len(logs))
	return nil
}

// Query retrieves audit logs based on query parameters
func (m *MockElasticsearchRepository) Query(ctx context.Context, query *ChangeLogQuery) (*ChangeLogQueryResult, error) {
	if query == nil {
		query = &ChangeLogQuery{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []DataChangeLog
	var total int64

	for _, log := range m.logs {
		if m.matchesQuery(log, query) {
			results = append(results, *log)
			total++
		}
	}

	// Apply limit and offset
	limit := query.Limit
	if limit == 0 {
		limit = 100
	}
	offset := query.Offset

	start := offset
	end := offset + limit
	if start > int(total) {
		start = int(total)
	}
	if end > int(total) {
		end = int(total)
	}

	var paginatedResults []DataChangeLog
	if start < end {
		paginatedResults = results[start:end]
	}

	return &ChangeLogQueryResult{
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		Records: paginatedResults,
	}, nil
}

// GetByPrimaryKey retrieves all changes for a specific entity by primary key
func (m *MockElasticsearchRepository) GetByPrimaryKey(ctx context.Context, domain, entity, primaryKey string, limit, offset int) (*ChangeLogQueryResult, error) {
	query := &ChangeLogQuery{
		Domain:        domain,
		Entity:        entity,
		PrimaryKeyStr: primaryKey,
		Limit:         limit,
		Offset:        offset,
	}

	return m.Query(ctx, query)
}

// GetEntityHistory retrieves the complete change history for an entity
func (m *MockElasticsearchRepository) GetEntityHistory(ctx context.Context, domain, entity, primaryKey string) (*EntityChangeHistory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	history := &EntityChangeHistory{
		Domain:        domain,
		Entity:        entity,
		PrimaryKeyStr: primaryKey,
		Changes:       []DataChangeLog{},
		ChangedByList: []string{},
		Operations:    make(map[string]int64),
	}

	userSet := make(map[string]bool)

	for _, log := range m.logs {
		if log.Domain == domain && log.Entity == entity && log.PrimaryKeyStr == primaryKey {
			history.Changes = append(history.Changes, *log)
			history.ChangeCount++

			// Track users
			if log.ChangedBy != "" {
				userSet[log.ChangedBy] = true
			}

			// Track operations
			history.Operations[log.Operation]++

			// Update timestamps
			if history.FirstChange.IsZero() || log.ChangeTimestamp.Before(history.FirstChange) {
				history.FirstChange = log.ChangeTimestamp
			}
			if log.ChangeTimestamp.After(history.LastChange) {
				history.LastChange = log.ChangeTimestamp
			}
		}
	}

	for user := range userSet {
		history.ChangedByList = append(history.ChangedByList, user)
	}

	return history, nil
}

// DeleteOlderThan deletes audit logs older than the specified date
func (m *MockElasticsearchRepository) DeleteOlderThan(ctx context.Context, domain, entity string, date time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	toDelete := []string{}
	for id, log := range m.logs {
		if log.Domain == domain && log.Entity == entity && log.ChangeTimestamp.Before(date) {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		delete(m.logs, id)
	}

	return nil
}

// GetStats returns statistics about audit logs
func (m *MockElasticsearchRepository) GetStats(ctx context.Context, domain, entity string, startDate, endDate time.Time) (*AuditStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &AuditStats{
		Domain:          domain,
		Entity:          entity,
		OperationCounts: make(map[string]int64),
		DateRange: DateRange{
			Start: startDate,
			End:   endDate,
		},
	}

	userSet := make(map[string]bool)
	entitySet := make(map[string]bool)
	var totalFields int64

	for _, log := range m.logs {
		if log.Domain == domain && log.Entity == entity &&
			log.ChangeTimestamp.After(startDate) && log.ChangeTimestamp.Before(endDate) {

			stats.TotalRecords++
			stats.OperationCounts[log.Operation]++

			if log.ChangedBy != "" {
				userSet[log.ChangedBy] = true
			}

			if log.PrimaryKeyStr != "" {
				entitySet[log.PrimaryKeyStr] = true
			}

			totalFields += int64(len(log.ChangeData))
		}
	}

	stats.UniqueUsers = int64(len(userSet))
	stats.UniqueEntities = int64(len(entitySet))

	if stats.TotalRecords > 0 {
		stats.AverageFieldsChanged = float64(totalFields) / float64(stats.TotalRecords)
	}

	return stats, nil
}

// Close closes the repository connection
func (m *MockElasticsearchRepository) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logs = make(map[string]*DataChangeLog)
	m.index = 0

	return nil
}

// Health checks if the repository is healthy and accessible
func (m *MockElasticsearchRepository) Health(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Mock repository is always healthy
	return nil
}

// matchesQuery checks if a log matches the query criteria
func (m *MockElasticsearchRepository) matchesQuery(log *DataChangeLog, query *ChangeLogQuery) bool {
	if query.Domain != "" && log.Domain != query.Domain {
		return false
	}

	if query.Entity != "" && log.Entity != query.Entity {
		return false
	}

	if query.PrimaryKeyStr != "" && log.PrimaryKeyStr != query.PrimaryKeyStr {
		return false
	}

	if query.Operation != "" && log.Operation != query.Operation {
		return false
	}

	if query.ChangedBy != "" && log.ChangedBy != query.ChangedBy {
		return false
	}

	if !query.StartDate.IsZero() && log.ChangeTimestamp.Before(query.StartDate) {
		return false
	}

	if !query.EndDate.IsZero() && log.ChangeTimestamp.After(query.EndDate) {
		return false
	}

	return true
}

// GetAllLogs returns all logs in the repository (for testing)
func (m *MockElasticsearchRepository) GetAllLogs() []DataChangeLog {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var logs []DataChangeLog
	for _, log := range m.logs {
		logs = append(logs, *log)
	}

	return logs
}

// ClearAllLogs clears all logs from the repository (for testing)
func (m *MockElasticsearchRepository) ClearAllLogs() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logs = make(map[string]*DataChangeLog)
	m.index = 0
}

// GetLogCount returns the number of logs in the repository
func (m *MockElasticsearchRepository) GetLogCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.logs)
}
