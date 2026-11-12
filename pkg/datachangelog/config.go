package datachangelog

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

// Config represents the complete audit logging configuration
type Config struct {
	Elasticsearch ElasticsearchConfig `yaml:"elasticsearch"`
	Entities      []EntityConfig      `yaml:"entities"`
	Global        GlobalConfig        `yaml:"global"`
}

// GlobalConfig represents global settings for audit logging
type GlobalConfig struct {
	Enabled           bool     `yaml:"enabled"`
	ExcludedFields    []string `yaml:"excluded_fields"`     // Fields to exclude from all entities
	SensitiveFields   []string `yaml:"sensitive_fields"`    // Fields to redact in all entities
	IncludeBeforeData bool     `yaml:"include_before_data"` // Include full before snapshot
	IncludeAfterData  bool     `yaml:"include_after_data"`  // Include full after snapshot
	IncludeIPAddress  bool     `yaml:"include_ip_address"`
	IncludeUserAgent  bool     `yaml:"include_user_agent"`
	MaxMetadataSize   int      `yaml:"max_metadata_size"` // Max size in bytes for metadata
}

// EntityConfig represents audit logging configuration for a specific entity
type EntityConfig struct {
	Domain            string            `yaml:"domain"` // Domain name (e.g., "appointment")
	Entity            string            `yaml:"entity"` // Entity name (e.g., "Appointment")
	Enabled           bool              `yaml:"enabled"`
	Operations        []string          `yaml:"operations"` // Operations to log: CREATE, UPDATE, DELETE
	PrimaryKey        PrimaryKeyConfig  `yaml:"primary_key"`
	ExcludedFields    []string          `yaml:"excluded_fields"`
	SensitiveFields   []string          `yaml:"sensitive_fields"`
	IncludeBeforeData bool              `yaml:"include_before_data"` // Override global setting
	IncludeAfterData  bool              `yaml:"include_after_data"`  // Override global setting
	Transformers      map[string]string `yaml:"transformers"`        // Field -> transformer name mapping
	Metadata          map[string]string `yaml:"metadata"`            // Custom metadata to include
}

// ElasticsearchConfig represents Elasticsearch connection and behavior configuration
type ElasticsearchConfig struct {
	Enabled            bool          `yaml:"enabled"`
	Addresses          []string      `yaml:"addresses"` // e.g., ["https://localhost:9200"]
	Username           string        `yaml:"username"`
	Password           string        `yaml:"password"`
	APIKey             string        `yaml:"api_key"` // Alternative to username/password
	InsecureSkipVerify bool          `yaml:"insecure_skip_verify"`
	CACert             string        `yaml:"ca_cert"`       // Path to CA certificate
	IndexPrefix        string        `yaml:"index_prefix"`  // e.g., "audit-log"
	IndexPattern       string        `yaml:"index_pattern"` // e.g., "audit-log-{domain}-{yyyy.MM}"
	NumWorkers         int           `yaml:"num_workers"`   // Number of async workers
	BulkSize           int           `yaml:"bulk_size"`     // Batch size for bulk operations
	MaxRetries         int           `yaml:"max_retries"`
	RetryDelay         time.Duration `yaml:"retry_delay"`
	FlushInterval      time.Duration `yaml:"flush_interval"`
	RequestTimeout     time.Duration `yaml:"request_timeout"`
}

// PrimaryKeyConfig defines how to extract primary keys from entities
type PrimaryKeyConfig struct {
	SingleKey     string   `yaml:"single_key"`     // Single key field name
	CompositeKeys []string `yaml:"composite_keys"` // Multiple key field names
	Separator     string   `yaml:"separator"`      // Separator for composite keys
}

// LoadConfig loads audit log configuration from YAML
func LoadConfig(configYAML []byte) (*Config, error) {
	var cfg Config

	// Set defaults
	cfg.setDefaults()

	// Parse YAML
	err := yaml.Unmarshal(configYAML, &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse audit log config: %w", err)
	}

	// Validate configuration
	err = cfg.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid audit log config: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets sensible defaults for the configuration
func (c *Config) setDefaults() {
	if c.Elasticsearch.NumWorkers == 0 {
		c.Elasticsearch.NumWorkers = 4
	}
	if c.Elasticsearch.BulkSize == 0 {
		c.Elasticsearch.BulkSize = 100
	}
	if c.Elasticsearch.MaxRetries == 0 {
		c.Elasticsearch.MaxRetries = 3
	}
	if c.Elasticsearch.RetryDelay == 0 {
		c.Elasticsearch.RetryDelay = 500 * time.Millisecond
	}
	if c.Elasticsearch.FlushInterval == 0 {
		c.Elasticsearch.FlushInterval = 2 * time.Second
	}
	if c.Elasticsearch.RequestTimeout == 0 {
		c.Elasticsearch.RequestTimeout = 10 * time.Second
	}
	if c.Elasticsearch.IndexPrefix == "" {
		c.Elasticsearch.IndexPrefix = "audit-log"
	}
	if c.Elasticsearch.IndexPattern == "" {
		c.Elasticsearch.IndexPattern = "{prefix}-{domain}-{yyyy.MM}"
	}
	if c.Global.MaxMetadataSize == 0 {
		c.Global.MaxMetadataSize = 10 * 1024 // 10KB
	}

	// Set default primary key separator
	for i := range c.Entities {
		if c.Entities[i].PrimaryKey.Separator == "" {
			c.Entities[i].PrimaryKey.Separator = ":"
		}
	}
}

// Validate performs validation checks on the configuration
func (c *Config) Validate() error {
	if !c.Elasticsearch.Enabled {
		return nil // Elasticsearch not configured, audit logging disabled
	}

	if len(c.Elasticsearch.Addresses) == 0 {
		return fmt.Errorf("elasticsearch addresses must be specified")
	}

	if c.Elasticsearch.Username == "" && c.Elasticsearch.APIKey == "" {
		return fmt.Errorf("elasticsearch authentication required: username/password or api_key")
	}

	for _, entity := range c.Entities {
		if entity.Entity == "" {
			return fmt.Errorf("entity name must be specified for all entities")
		}

		if entity.Domain == "" {
			return fmt.Errorf("domain must be specified for entity %s", entity.Entity)
		}

		pkCfg := entity.PrimaryKey
		if pkCfg.SingleKey == "" && len(pkCfg.CompositeKeys) == 0 {
			return fmt.Errorf("primary key configuration required for entity %s.%s", entity.Domain, entity.Entity)
		}

		if pkCfg.SingleKey != "" && len(pkCfg.CompositeKeys) > 0 {
			return fmt.Errorf("only single_key or composite_keys can be specified, not both, for entity %s.%s", entity.Domain, entity.Entity)
		}
	}

	return nil
}

// GetEntity retrieves entity configuration by domain and entity name
func (c *Config) GetEntity(domain, entity string) *EntityConfig {
	for i := range c.Entities {
		if c.Entities[i].Domain == domain && c.Entities[i].Entity == entity {
			return &c.Entities[i]
		}
	}
	return nil
}

// IsEntityEnabled checks if an entity is enabled for audit logging
func (c *Config) IsEntityEnabled(domain, entity string) bool {
	if !c.Elasticsearch.Enabled {
		return false
	}

	entityCfg := c.GetEntity(domain, entity)
	if entityCfg == nil {
		return false
	}

	return entityCfg.Enabled
}

// IsOperationEnabled checks if a specific operation is enabled for an entity
func (c *Config) IsOperationEnabled(domain, entity, operation string) bool {
	entityCfg := c.GetEntity(domain, entity)
	if entityCfg == nil {
		return false
	}

	for _, op := range entityCfg.Operations {
		if op == operation {
			return true
		}
	}

	return false
}

// GetIndexName generates the Elasticsearch index name for a given domain and date
func (c *Config) GetIndexName(domain string, timestamp time.Time) string {
	pattern := c.Elasticsearch.IndexPattern
	prefix := c.Elasticsearch.IndexPrefix

	// Replace placeholders
	pattern = replacePattern(pattern, "{prefix}", prefix)
	pattern = replacePattern(pattern, "{domain}", domain)
	pattern = replacePattern(pattern, "{yyyy}", fmt.Sprintf("%04d", timestamp.Year()))
	pattern = replacePattern(pattern, "{MM}", fmt.Sprintf("%02d", timestamp.Month()))
	pattern = replacePattern(pattern, "{dd}", fmt.Sprintf("%02d", timestamp.Day()))

	return pattern
}

// replacePattern is a helper function to replace placeholders in index pattern
func replacePattern(pattern, placeholder, replacement string) string {
	for pattern != strings.ReplaceAll(pattern, placeholder, replacement) {
		pattern = strings.ReplaceAll(pattern, placeholder, replacement)
	}
	return pattern
}

// MergeEntityConfig merges global settings with entity-specific settings
func (c *Config) MergeEntityConfig(entityCfg *EntityConfig) *MergedEntityConfig {
	merged := &MergedEntityConfig{
		Domain:            entityCfg.Domain,
		Entity:            entityCfg.Entity,
		Enabled:           entityCfg.Enabled,
		Operations:        entityCfg.Operations,
		PrimaryKey:        entityCfg.PrimaryKey,
		ExcludedFields:    append(c.Global.ExcludedFields, entityCfg.ExcludedFields...),
		SensitiveFields:   append(c.Global.SensitiveFields, entityCfg.SensitiveFields...),
		IncludeBeforeData: entityCfg.IncludeBeforeData,
		IncludeAfterData:  entityCfg.IncludeAfterData,
		Transformers:      entityCfg.Transformers,
		Metadata:          entityCfg.Metadata,
	}

	// Apply global overrides if entity doesn't specify
	if !entityCfg.IncludeBeforeData && c.Global.IncludeBeforeData {
		merged.IncludeBeforeData = true
	}
	if !entityCfg.IncludeAfterData && c.Global.IncludeAfterData {
		merged.IncludeAfterData = true
	}

	return merged
}

// MergedEntityConfig represents entity configuration with merged global settings
type MergedEntityConfig struct {
	Domain            string
	Entity            string
	Enabled           bool
	Operations        []string
	PrimaryKey        PrimaryKeyConfig
	ExcludedFields    []string
	SensitiveFields   []string
	IncludeBeforeData bool
	IncludeAfterData  bool
	Transformers      map[string]string
	Metadata          map[string]string
}
