package datachangelog

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
)

// SetupAuditInfrastructure initializes the complete audit logging infrastructure
// It:
// 1. Loads and parses the datachangelog configuration from YAML
// 2. Creates the storage repository:
//   - Elasticsearch only        (elasticsearch.enabled: true)
//   - ClickHouse only           (clickhouse.enabled: true)
//   - Both (dual-write)         (both enabled; ES stays the source of truth for reads)
//
// 3. Initializes sanitizers and diff calculators
// 4. Returns the gRPC unary interceptor for audit logging
//
// Args:
//   - configFilePath: Path to the audit configuration YAML file (e.g., "config/datachangelog_config.yaml")
//
// Returns:
//   - grpc.UnaryServerInterceptor: The audit interceptor to be added to the gRPC server chain
//   - error: Any error during initialization
//
// Example:
//
//	auditInterceptor, err := datachangelog.SetupAuditInfrastructure("config/datachangelog_config.yaml")
//	if err != nil {
//		log.Fatalf("Failed to setup audit infrastructure: %v", err)
//	}
//	// Add to gRPC server chain
func SetupAuditInfrastructure(configFilePath string) (grpc.UnaryServerInterceptor, error) {
	// 1. Load configuration from YAML file with environment variable substitution
	configYAML, err := loadAndProcessConfigYAML(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load audit config: %w", err)
	}

	// 2. Parse the configuration
	auditConfig, err := LoadConfig(configYAML)
	if err != nil {
		return nil, fmt.Errorf("failed to parse audit config: %w", err)
	}

	// 3. Check which storage backends are enabled
	esEnabled := auditConfig.Elasticsearch.Enabled
	chEnabled := auditConfig.ClickHouse.Enabled

	if !esEnabled && !chEnabled {
		fmt.Println("[AUDIT] Audit logging (Elasticsearch/ClickHouse) is disabled in configuration")
		return createNoOpInterceptor(), nil
	}

	var repo Repository

	// 4. Elasticsearch repository (existing logic, unchanged)
	if esEnabled {
		// Validate that addresses are available
		if len(auditConfig.Elasticsearch.Addresses) == 0 {
			fmt.Println("[AUDIT] Warning: No Elasticsearch addresses configured")
			if !chEnabled {
				fmt.Println("[AUDIT] Using mock repository")
				return createAuditInterceptorWithMock(), nil
			}
			fmt.Println("[AUDIT] Skipping Elasticsearch, ClickHouse is enabled")
		} else {
			esRepo, err := NewElasticsearchRepository(&auditConfig.Elasticsearch)
			if err != nil {
				// Check if it's an authorization error - if so, still use the repo
				// because the user might have index-specific permissions even without cluster monitor
				if strings.Contains(err.Error(), "security_exception") || strings.Contains(err.Error(), "unauthorized") {
					fmt.Printf("[AUDIT] ⚠ Authorization warning: %v\n", err)
					fmt.Println("[AUDIT] User may not have cluster monitor privilege, but continuing with Elasticsearch repository...")
					fmt.Println("[AUDIT] ℹ To fix: Grant 'monitor' cluster privilege to jecis-log-user role")
					repo = esRepo
				} else {
					fmt.Printf("[AUDIT] Warning: Failed to create elasticsearch repository: %v\n", err)
					fmt.Println("[AUDIT] Continuing with fallback to mock repository...")
					repo = NewMockElasticsearchRepository()
				}
			} else {
				// Verify Elasticsearch connectivity
				healthCtx, healthCancel := context.WithTimeout(context.Background(), 5*time.Second)
				healthErr := esRepo.Health(healthCtx)
				healthCancel()

				if healthErr != nil {
					// Check if it's a permission error
					if strings.Contains(healthErr.Error(), "security_exception") || strings.Contains(healthErr.Error(), "unauthorized") || strings.Contains(healthErr.Error(), "403") {
						fmt.Printf("[AUDIT] ⚠ Authorization warning: %v\n", healthErr)
						fmt.Println("[AUDIT] User does not have cluster monitor privilege")
						fmt.Println("[AUDIT] ✓ Continuing with Elasticsearch repository (index operations should work)")
						fmt.Println("[AUDIT] ℹ To fix permanently: Grant 'monitor' cluster privilege to jecis-log-user role in Elasticsearch")
						repo = esRepo
					} else {
						fmt.Printf("[AUDIT] Warning: Elasticsearch health check failed: %v\n", healthErr)
						fmt.Println("[AUDIT] Continuing with fallback to mock repository...")
						esRepo.Close()
						repo = NewMockElasticsearchRepository()
					}
				} else {
					fmt.Printf("[AUDIT] ✓ Elasticsearch connection successful\n")
					repo = esRepo
				}
			}
		}
	}

	// 5. ClickHouse repository
	//    - ES + ClickHouse enabled -> dual-write (ES primary for reads, ClickHouse secondary)
	//    - ClickHouse only         -> ClickHouse is the only storage
	//    A ClickHouse failure never stops the app: it is logged and audit continues without it.
	if chEnabled {
		chRepo, chErr := NewClickHouseRepository(&auditConfig.ClickHouse)
		switch {
		case chErr != nil:
			fmt.Printf("[AUDIT] Warning: Failed to create clickhouse repository: %v\n", chErr)
		case repo == nil:
			fmt.Println("[AUDIT] ✓ ClickHouse connection successful (ClickHouse only)")
			repo = chRepo
		default:
			fmt.Println("[AUDIT] ✓ ClickHouse connection successful (dual-write with Elasticsearch)")
			repo = &DualWriteRepository{Primary: repo, Secondary: chRepo}
		}
	}

	// No usable storage at all -> fallback to mock, same behavior as before
	if repo == nil {
		fmt.Println("[AUDIT] Warning: No audit storage available, falling back to mock repository")
		repo = NewMockElasticsearchRepository()
	}

	// 6. Initialize sanitizer with merged sensitive fields
	sensitiveFields := auditConfig.Global.SensitiveFields
	sanitizer := NewSanitizer(sensitiveFields)

	// 7. Initialize diff calculator
	diffCalculator := NewDiffCalculator(auditConfig.Global.ExcludedFields, sensitiveFields)

	// 8. Create and return the interceptor
	interceptorCfg := &InterceptorConfig{
		Enabled:           esEnabled || chEnabled,
		Config:            auditConfig,
		Repository:        repo,
		Sanitizer:         sanitizer,
		DiffCalculator:    diffCalculator,
		CaptureBeforeData: auditConfig.Global.IncludeBeforeData,
		CaptureAfterData:  auditConfig.Global.IncludeAfterData,
		IncludePayload:    auditConfig.Global.IncludeAfterData,
		ExcludedMethods:   make(map[string]bool),
		IncludedMethods:   make(map[string]bool),
		UserExtractor:     &DefaultUserExtractor{},
		IPExtractor:       &DefaultIPExtractor{},
	}

	fmt.Println("[AUDIT] ✓ Audit infrastructure initialized successfully")
	return NewAuditInterceptor(interceptorCfg), nil
}

// loadAndProcessConfigYAML loads the configuration YAML file and substitutes environment variables
func loadAndProcessConfigYAML(configFilePath string) ([]byte, error) {
	// Read the configuration file
	data, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configFilePath, err)
	}

	// Convert to string for processing
	configStr := string(data)

	// Replace environment variable placeholders like ${VAR_NAME}
	// This supports patterns like "${ELASTIC_URL}", "${CLICKHOUSE_ADDR}", etc.
	configStr = os.ExpandEnv(configStr)

	// Additional manual replacement for common patterns
	// This ensures robustness even if os.ExpandEnv doesn't fully work
	for {
		before := configStr
		configStr = replaceEnvVariable(configStr, "ELASTIC_URL")
		configStr = replaceEnvVariable(configStr, "ELASTIC_USER")
		configStr = replaceEnvVariable(configStr, "ELASTIC_PASSWORD")

		// Stop if no more replacements
		if configStr == before {
			break
		}
	}

	fmt.Printf("[AUDIT] Configuration loaded from %s\n", configFilePath)
	return []byte(configStr), nil
}

// replaceEnvVariable replaces ${VAR_NAME} patterns with environment variable values
func replaceEnvVariable(configStr, envVarName string) string {
	pattern := "${" + envVarName + "}"
	if strings.Contains(configStr, pattern) {
		value := os.Getenv(envVarName)
		configStr = strings.ReplaceAll(configStr, pattern, value)
		if value != "" {
			fmt.Printf("[AUDIT] ✓ Substituted environment variable: %s\n", envVarName)
		} else {
			fmt.Printf("[AUDIT] Warning: Environment variable %s is empty\n", envVarName)
		}
	}
	return configStr
}

// createNoOpInterceptor creates a no-operation interceptor (audit disabled)
func createNoOpInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return handler(ctx, req)
	}
}

// createAuditInterceptorWithMock creates an interceptor with the mock repository
func createAuditInterceptorWithMock() grpc.UnaryServerInterceptor {
	mockRepo := NewMockElasticsearchRepository()

	interceptorCfg := &InterceptorConfig{
		Enabled:           true,
		Config:            nil,
		Repository:        mockRepo,
		Sanitizer:         NewSanitizer([]string{}),
		DiffCalculator:    NewDiffCalculator([]string{}, []string{}),
		CaptureBeforeData: true,
		CaptureAfterData:  true,
		IncludePayload:    true,
		ExcludedMethods:   make(map[string]bool),
		IncludedMethods:   make(map[string]bool),
		UserExtractor:     &DefaultUserExtractor{},
		IPExtractor:       &DefaultIPExtractor{},
	}

	fmt.Println("[AUDIT] ✓ Using mock repository for audit logging")
	return NewAuditInterceptor(interceptorCfg)
}

// StandaloneAuditInterceptor creates an interceptor for testing/development without full file initialization
// This is useful if you want to add audit logging without fully initializing from a configuration file
//
// Args:
//   - mockRepo: A Repository implementation (can use NewMockElasticsearchRepository() for testing)
//
// Returns:
//   - grpc.UnaryServerInterceptor: A fully functional audit interceptor
//
// Example:
//
//	mockRepo := datachangelog.NewMockElasticsearchRepository()
//	interceptor := datachangelog.StandaloneAuditInterceptor(mockRepo)
func StandaloneAuditInterceptor(mockRepo Repository) grpc.UnaryServerInterceptor {
	if mockRepo == nil {
		mockRepo = NewMockElasticsearchRepository()
	}

	interceptorCfg := &InterceptorConfig{
		Enabled:           true,
		Config:            nil,
		Repository:        mockRepo,
		Sanitizer:         NewSanitizer([]string{}),
		DiffCalculator:    NewDiffCalculator([]string{}, []string{}),
		CaptureBeforeData: true,
		CaptureAfterData:  true,
		IncludePayload:    true,
		ExcludedMethods:   make(map[string]bool),
		IncludedMethods:   make(map[string]bool),
		UserExtractor:     &DefaultUserExtractor{},
		IPExtractor:       &DefaultIPExtractor{},
	}

	return NewAuditInterceptor(interceptorCfg)
}
