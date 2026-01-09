package datachangelog

import (
	"context"
	"time"
)

// Repository defines the interface for storing and retrieving audit logs
type Repository interface {
	// Save persists a single data change log entry
	Save(ctx context.Context, log *DataChangeLog) error

	// SaveBatch persists multiple data change log entries in a single operation
	SaveBatch(ctx context.Context, logs []DataChangeLog) error

	// Query retrieves audit logs based on query parameters
	Query(ctx context.Context, query *ChangeLogQuery) (*ChangeLogQueryResult, error)

	// GetByPrimaryKey retrieves all changes for a specific entity by primary key
	GetByPrimaryKey(ctx context.Context, domain, entity, primaryKey string, limit, offset int) (*ChangeLogQueryResult, error)

	// GetEntityHistory retrieves the complete change history for an entity
	GetEntityHistory(ctx context.Context, domain, entity, primaryKey string) (*EntityChangeHistory, error)

	// DeleteOlderThan deletes audit logs older than the specified date
	DeleteOlderThan(ctx context.Context, domain, entity string, date time.Time) error

	// GetStats returns statistics about audit logs
	GetStats(ctx context.Context, domain, entity string, startDate, endDate time.Time) (*AuditStats, error)

	// Close closes the repository connection/resources
	Close() error

	// Health checks if the repository is healthy and accessible
	Health(ctx context.Context) error
}

// AuditStats represents statistics about audit logs
type AuditStats struct {
	Domain               string           `json:"domain"`
	Entity               string           `json:"entity"`
	TotalRecords         int64            `json:"total_records"`
	DateRange            DateRange        `json:"date_range"`
	OperationCounts      map[string]int64 `json:"operation_counts"` // CREATE, UPDATE, DELETE counts
	UniqueUsers          int64            `json:"unique_users"`
	UniqueEntities       int64            `json:"unique_entities"`
	AverageFieldsChanged float64          `json:"average_fields_changed"`
}

// DateRange represents a range of dates
type DateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// CacheRepository is an optional interface for repositories that support caching
type CacheRepository interface {
	Repository

	// InvalidateCache clears the cache for a specific entity
	InvalidateCache(ctx context.Context, domain, entity, primaryKey string) error

	// ClearCache clears all cached data
	ClearCache(ctx context.Context) error
}

// BatchWriter handles asynchronous batch writing of audit logs
type BatchWriter interface {
	// Write queues a log entry for batch writing
	Write(log *DataChangeLog) error

	// Flush writes all queued entries immediately
	Flush(ctx context.Context) error

	// Close gracefully closes the writer, flushing any pending writes
	Close() error

	// Status returns the current status of the writer
	Status() BatchWriterStatus
}

// BatchWriterStatus represents the current status of a batch writer
type BatchWriterStatus struct {
	IsRunning        bool
	QueueSize        int
	ProcessedCount   int64
	FailedCount      int64
	LastFlushTime    time.Time
	AverageLatencyMs float64
}

// QueryBuilder helps construct complex queries for audit logs
type QueryBuilder interface {
	// Domain sets the domain filter
	Domain(domain string) QueryBuilder

	// Entity sets the entity filter
	Entity(entity string) QueryBuilder

	// PrimaryKey sets the primary key filter
	PrimaryKey(key string) QueryBuilder

	// Operation sets the operation filter (CREATE, UPDATE, DELETE)
	Operation(op string) QueryBuilder

	// User sets the user filter
	User(userID string) QueryBuilder

	// DateRange sets the date range filter
	DateRange(start, end time.Time) QueryBuilder

	// Action sets the action/handler filter
	Action(action string) QueryBuilder

	// Limit sets the result limit
	Limit(limit int) QueryBuilder

	// Offset sets the result offset (for pagination)
	Offset(offset int) QueryBuilder

	// Build constructs the final query
	Build() *ChangeLogQuery

	// Reset resets the builder to its initial state
	Reset() QueryBuilder
}

// AuditTrail represents a complete audit trail for an entity
type AuditTrail struct {
	Domain         string         `json:"domain"`
	Entity         string         `json:"entity"`
	PrimaryKey     string         `json:"primary_key"`
	CreatedAt      time.Time      `json:"created_at"`
	CreatedBy      string         `json:"created_by"`
	LastModifiedAt time.Time      `json:"last_modified_at"`
	LastModifiedBy string         `json:"last_modified_by"`
	Modifications  []Modification `json:"modifications"`
	ChangeCount    int            `json:"change_count"`
}

// Modification represents a single modification in the audit trail
type Modification struct {
	Timestamp      time.Time              `json:"timestamp"`
	Operation      string                 `json:"operation"` // CREATE, UPDATE, DELETE
	ModifiedBy     string                 `json:"modified_by"`
	Changes        []FieldDiff            `json:"changes"`
	BeforeSnapshot map[string]interface{} `json:"before_snapshot,omitempty"`
	AfterSnapshot  map[string]interface{} `json:"after_snapshot,omitempty"`
}

// ExportFormat defines the format for exporting audit logs
type ExportFormat string

const (
	ExportFormatJSON ExportFormat = "json"
	ExportFormatCSV  ExportFormat = "csv"
	ExportFormatXML  ExportFormat = "xml"
)

// Exporter handles exporting audit logs in various formats
type Exporter interface {
	// Export exports logs to a specific format
	Export(ctx context.Context, query *ChangeLogQuery, format ExportFormat) ([]byte, error)

	// ExportToFile exports logs to a file
	ExportToFile(ctx context.Context, query *ChangeLogQuery, format ExportFormat, filePath string) error
}

// ComplianceReport represents a compliance audit report
type ComplianceReport struct {
	ReportID         string           `json:"report_id"`
	GeneratedAt      time.Time        `json:"generated_at"`
	Domain           string           `json:"domain"`
	Entity           string           `json:"entity"`
	DateRange        DateRange        `json:"date_range"`
	TotalChanges     int64            `json:"total_changes"`
	UserActivity     map[string]int64 `json:"user_activity"`
	OperationSummary map[string]int64 `json:"operation_summary"`
	RiskIndicators   []RiskIndicator  `json:"risk_indicators"`
}

// RiskIndicator represents a potential compliance or security risk
type RiskIndicator struct {
	Level       string                 `json:"level"` // HIGH, MEDIUM, LOW
	Description string                 `json:"description"`
	Details     map[string]interface{} `json:"details"`
}

// AuditLogger is a high-level interface for audit logging operations
type AuditLogger interface {
	// LogCreate logs a create operation
	LogCreate(ctx context.Context, domain, entity string, primaryKey string, data map[string]interface{}, metadata map[string]string) error

	// LogUpdate logs an update operation
	LogUpdate(ctx context.Context, domain, entity string, primaryKey string, before, after map[string]interface{}, metadata map[string]string) error

	// LogDelete logs a delete operation
	LogDelete(ctx context.Context, domain, entity string, primaryKey string, data map[string]interface{}, metadata map[string]string) error

	// GetAuditTrail retrieves the complete audit trail for an entity
	GetAuditTrail(ctx context.Context, domain, entity, primaryKey string) (*AuditTrail, error)

	// GenerateComplianceReport generates a compliance report for a domain/entity
	GenerateComplianceReport(ctx context.Context, domain, entity string, startDate, endDate time.Time) (*ComplianceReport, error)
}
