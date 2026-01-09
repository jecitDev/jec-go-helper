package datachangelog

import (
	"time"
)

// DataChangeLog represents a single data change event
type DataChangeLog struct {
	ID            string                 `json:"id"`
	Domain        string                 `json:"domain"`
	Entity        string                 `json:"entity"`
	Operation     string                 `json:"operation"` // CREATE, UPDATE, DELETE
	PrimaryKey    map[string]interface{} `json:"primary_key"`
	PrimaryKeyStr string                 `json:"primary_key_str"` // Composite key as string
	ChangeData    map[string]interface{} `json:"change_data"`
	AfterData     map[string]interface{} `json:"after_data"`
	// Changes         []FieldDiff            `json:"changes"`
	// ChangesRaw      string                 `json:"changes_raw"`
	ChangedBy       string                 `json:"changed_by"`       // User ID or username
	ChangedByEmail  string                 `json:"changed_by_email"` // User email
	ChangeTimestamp time.Time              `json:"change_timestamp"`
	RequestID       string                 `json:"request_id"` // Trace ID
	IPAddress       string                 `json:"ip_address"`
	UserAgent       string                 `json:"user_agent"`
	Metadata        map[string]interface{} `json:"metadata"` // Additional custom metadata
}

// FieldDiff represents a change in a single field
type FieldDiff struct {
	FieldName   string      `json:"field_name"`
	FieldType   string      `json:"field_type"` // string, number, boolean, date, etc.
	OldValue    interface{} `json:"old_value"`
	NewValue    interface{} `json:"new_value"`
	Sanitized   bool        `json:"sanitized"`             // Indicates if value was sanitized
	Transformer string      `json:"transformer,omitempty"` // Transformer applied if any
}

// ChangeLogQuery represents query parameters for retrieving audit logs
type ChangeLogQuery struct {
	Domain        string
	Entity        string
	PrimaryKey    map[string]interface{}
	PrimaryKeyStr string
	Operation     string
	ChangedBy     string
	StartDate     time.Time
	EndDate       time.Time
	Limit         int
	Offset        int
}

// ChangeLogQueryResult wraps query results with metadata
type ChangeLogQueryResult struct {
	Total   int64           `json:"total"`
	Limit   int             `json:"limit"`
	Offset  int             `json:"offset"`
	Records []DataChangeLog `json:"records"`
}

// EntityChangeHistory represents the full history of an entity
type EntityChangeHistory struct {
	Domain        string                 `json:"domain"`
	Entity        string                 `json:"entity"`
	PrimaryKey    map[string]interface{} `json:"primary_key"`
	PrimaryKeyStr string                 `json:"primary_key_str"`
	ChangeCount   int64                  `json:"change_count"`
	FirstChange   time.Time              `json:"first_change"`
	LastChange    time.Time              `json:"last_change"`
	ChangedByList []string               `json:"changed_by_list"` // Unique list of users who made changes
	Operations    map[string]int64       `json:"operations"`      // Count of each operation type
	Changes       []DataChangeLog        `json:"changes"`
}
