package datachangelog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// InterceptorConfig holds configuration for the audit logging interceptor
type InterceptorConfig struct {
	Enabled           bool
	Config            *Config
	Repository        Repository
	Sanitizer         *Sanitizer
	DiffCalculator    *DiffCalculator
	CaptureBeforeData bool
	CaptureAfterData  bool
	IncludePayload    bool
	ExcludedMethods   map[string]bool
	IncludedMethods   map[string]bool // If non-empty, only these methods are logged
	UserExtractor     UserExtractor
	IPExtractor       IPExtractor
}

// UserExtractor defines how to extract user information from context
type UserExtractor interface {
	ExtractUser(ctx context.Context) (userID, email, role string, err error)
}

// IPExtractor defines how to extract IP address from context
type IPExtractor interface {
	ExtractIP(ctx context.Context) string
}

// DefaultUserExtractor implements UserExtractor
type DefaultUserExtractor struct{}

func (due *DefaultUserExtractor) ExtractUser(ctx context.Context) (userID, email, role string, err error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", "", "", nil
	}

	// Extract from metadata headers (customize based on your auth implementation)
	if values := md.Get("user-id"); len(values) > 0 {
		userID = values[0]
	}
	if values := md.Get("user-email"); len(values) > 0 {
		email = values[0]
	}
	if values := md.Get("user-role"); len(values) > 0 {
		role = values[0]
	}

	return
}

// DefaultIPExtractor implements IPExtractor
type DefaultIPExtractor struct{}

func (die *DefaultIPExtractor) ExtractIP(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	if values := md.Get("x-forwarded-for"); len(values) > 0 {
		return values[0]
	}

	return ""
}

// NewAuditInterceptor creates a new gRPC unary interceptor for audit logging
func NewAuditInterceptor(cfg *InterceptorConfig) grpc.UnaryServerInterceptor {
	if !cfg.Enabled || cfg.Repository == nil {
		// Return a no-op interceptor if disabled
		return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			return handler(ctx, req)
		}
	}

	// Set defaults
	if cfg.UserExtractor == nil {
		cfg.UserExtractor = &DefaultUserExtractor{}
	}
	if cfg.IPExtractor == nil {
		cfg.IPExtractor = &DefaultIPExtractor{}
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Parse full method name
		// Format: /appointment.AppointmentService/Add
		// After split by "/": ["", "appointment.AppointmentService", "Add"]
		method := info.FullMethod
		parts := strings.Split(method, "/")
		if len(parts) < 3 {
			return handler(ctx, req)
		}

		// Extract domain from package (e.g., "appointment.AppointmentService" -> "appointment")
		fullDomain := strings.ToLower(parts[1])
		domain := strings.Split(fullDomain, ".")[0] // Get the first part before the dot
		methodName := parts[2]
		entity := capitalizeFirstLetter(domain)

		// Check if this method should be logged
		if !shouldLogMethod(cfg, domain, methodName) {
			return handler(ctx, req)
		}
		fmt.Println("Test Debug")

		// Generate request ID
		requestID := generateRequestID()
		ctx = context.WithValue(ctx, "request-id", requestID)

		// Extract user information
		userID, userEmail, _, _ := cfg.UserExtractor.ExtractUser(ctx)

		// Extract IP address
		ipAddress := cfg.IPExtractor.ExtractIP(ctx)

		// Capture request data
		var reqData map[string]interface{}
		if cfg.IncludePayload {
			reqData = protoToMap(req)
		}

		// Call the handler
		startTime := time.Now()
		resp, err := handler(ctx, req)
		if err != nil {
			return resp, err
		}

		duration := time.Since(startTime)

		// Capture response data
		var respData map[string]interface{}
		if cfg.IncludePayload && resp != nil {
			respData = protoToMap(resp)
		}

		// Determine operation type from method name
		operation := inferOperation(methodName)

		// Extract primary key from request using the configured primary key fields
		primaryKey := extractPrimaryKey(cfg, domain, methodName, req, resp)

		// Skip logging if primary key is not found
		if primaryKey == "" {
			return resp, err
		}

		// Determine before/after data based on operation type
		beforeData := reqData
		afterData := respData

		switch operation {
		case "CREATE":
			beforeData = nil
		case "PATCH", "RESCHEDULE":
			beforeData = normalizePatchChangeData(reqData)
		case "DELETE":
			afterData = nil
		}

		// Create audit log entry
		auditLog := &DataChangeLog{
			ID:            uuid.New().String(),
			Domain:        domain,
			Entity:        methodName,
			Operation:     operation,
			PrimaryKey:    parsePrimaryKey(primaryKey),
			PrimaryKeyStr: primaryKey,
			ChangeData:    beforeData,
			AfterData:     afterData,
			// Changes:       []FieldDiff{},
			// ChangesRaw:      string(changesJSON),
			ChangedBy:       userID,
			ChangedByEmail:  userEmail,
			ChangeTimestamp: time.Now(),
			RequestID:       requestID,
			IPAddress:       ipAddress,
			UserAgent:       extractUserAgent(ctx),
			Metadata: map[string]interface{}{
				"method":   methodName,
				"duration": duration.Milliseconds(),
			},
		}

		// Add error information if operation failed
		if err != nil {
			auditLog.Metadata["error"] = err.Error()
			if st, ok := status.FromError(err); ok {
				auditLog.Metadata["grpc_code"] = st.Code().String()
				auditLog.Metadata["grpc_message"] = st.Message()
			}
		}

		// Sanitize sensitive fields
		if cfg.Sanitizer != nil && auditLog.AfterData != nil {
			entityCfg := cfg.Config.GetEntity(domain, entity)
			if entityCfg != nil {
				auditLog.AfterData = cfg.Sanitizer.SanitizeMap(auditLog.AfterData, entityCfg.ExcludedFields, entityCfg.SensitiveFields)
				auditLog.ChangeData = cfg.Sanitizer.SanitizeMap(auditLog.ChangeData, entityCfg.ExcludedFields, entityCfg.SensitiveFields)
			}
		}

		// Save to repository asynchronously
		go func() {
			saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := cfg.Repository.Save(saveCtx, auditLog); err != nil {
				// Log error but don't fail the request
				fmt.Printf("failed to save audit log: %v\n", err)
			}
		}()

		return resp, err
	}
}

// shouldLogMethod determines if a method should be logged
func shouldLogMethod(cfg *InterceptorConfig, domain, methodName string) bool {
	// If no config, don't log anything
	if cfg.Config == nil {
		return false
	}

	// Check excluded methods
	if cfg.ExcludedMethods[domain+"."+methodName] {
		return false
	}

	// Check included methods (if specified)
	if len(cfg.IncludedMethods) > 0 {
		return cfg.IncludedMethods[domain+"."+methodName]
	}

	// Find matching entity configuration by domain
	// We need to search through all entities to find one matching this domain
	var entityCfg *EntityConfig
	for i := range cfg.Config.Entities {
		if cfg.Config.Entities[i].Domain == domain {
			entityCfg = &cfg.Config.Entities[i]
			break
		}
	}

	if entityCfg == nil {
		return false
	}

	// Check if entity is enabled
	if !entityCfg.Enabled {
		return false
	}

	// Check if the operation (inferred from method name) is in the allowed operations list
	operation := inferOperation(methodName)
	for _, op := range entityCfg.Operations {
		if op == operation {
			return true
		}
	}

	return false
}

// inferOperation infers the operation type from the method name
func inferOperation(methodName string) string {
	lower := strings.ToLower(methodName)

	if strings.Contains(lower, "create") || strings.Contains(lower, "add") || strings.Contains(lower, "insert") {
		return "CREATE"
	}
	if strings.Contains(lower, "delete") || strings.Contains(lower, "remove") {
		return "DELETE"
	}
	if strings.Contains(lower, "void") {
		return "VOID"
	}
	if strings.Contains(lower, "patch") {
		return "PATCH"
	}
	if strings.Contains(lower, "reschedule") {
		return "RESCHEDULE"
	}
	if strings.Contains(lower, "update") || strings.Contains(lower, "modify") || strings.Contains(lower, "edit") {
		return "UPDATE"
	}

	return "OTHER"
}

// extractPrimaryKey extracts the primary key from the request using the configured primary key fields
// It supports both single_key and composite_keys configurations from the YAML file
func extractPrimaryKey(cfg *InterceptorConfig, domain, methodName string, req interface{}, resp interface{}) string {
	if req == nil {
		return ""
	}

	// Get entity configuration from the domain
	var entityCfg *EntityConfig
	if cfg != nil && cfg.Config != nil {
		for i := range cfg.Config.Entities {
			if cfg.Config.Entities[i].Domain == domain {
				entityCfg = &cfg.Config.Entities[i]
				break
			}
		}
	}

	// Convert proto message to map
	// dataMap := protoToMap(req)
	// if dataMap == nil {
	// 	if resp != nil {
	// 		respMap, ok := resp.(map[string]interface{})
	// 		if ok && respMap != nil {
	// 			dataMap = respMap
	// 		} else {
	// 			return ""
	// 		}
	// 	} else {
	// 		return ""
	// 	}
	// } else {
	// 	if resp != nil {
	// 		respMap, ok := resp.(map[string]interface{})
	// 		if ok && respMap != nil {
	// 			dataMap = mergeMaps(dataMap, respMap)
	// 		}
	// 	}
	// }
	// log.Println("Data Map for primary key extraction:", dataMap)

	// Convert request & response proto messages to map
	reqMap := protoToMap(req)
	respMap := protoToMap(resp)

	// Merge request + response (response wins)
	dataMap := map[string]interface{}{}

	if reqMap != nil {
		dataMap = reqMap
	}
	if respMap != nil {
		dataMap = mergeMaps(dataMap, respMap)
	}

	if len(dataMap) == 0 {
		return ""
	}

	// If entity config exists, use the configured primary key fields
	if entityCfg != nil && entityCfg.PrimaryKey.SingleKey != "" {
		// Single key configuration
		if val, ok := dataMap[entityCfg.PrimaryKey.SingleKey]; ok && val != nil {
			return fmt.Sprintf("%v", val)
		}
	}

	// Check for composite keys
	if entityCfg != nil && len(entityCfg.PrimaryKey.CompositeKeys) > 0 {
		// Extract all composite key values
		var keyValues []string
		for _, keyName := range entityCfg.PrimaryKey.CompositeKeys {
			if val, ok := dataMap[keyName]; ok && val != nil {
				keyValues = append(keyValues, fmt.Sprintf("%v", val))
			} else {
				return ""
			}
		}

		// Combine composite keys with separator
		if len(keyValues) > 0 {
			separator := entityCfg.PrimaryKey.Separator
			if separator == "" {
				separator = ":"
			}
			return strings.Join(keyValues, separator)
		}
	}

	// Fallback: try common primary key names if no config is available
	// primaryKeyNames := []string{"id", "ID", "identifier", "pk", "key", "appointment_no", "no"}

	// for _, keyName := range primaryKeyNames {
	// 	if val, ok := dataMap[keyName]; ok && val != nil {
	// 		return fmt.Sprintf("%v", val)
	// 	}
	// }

	return ""
}
func mergeMaps(m1, m2 map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy m1
	for k, v := range m1 {
		result[k] = v
	}

	// Copy m2 (overwrites m1 if keys overlap)
	for k, v := range m2 {
		result[k] = v
	}

	return result
}

// parsePrimaryKey converts a string primary key to a map
// For composite keys, it can parse the separator to create a structured map
func parsePrimaryKey(pkStr string) map[string]interface{} {
	if pkStr == "" {
		return nil
	}

	// For simple primary keys, create a basic map
	return map[string]interface{}{
		"value": pkStr,
	}
}

// protoToMap converts a protobuf message to a map
func protoToMap(msg interface{}) map[string]interface{} {
	if msg == nil {
		return nil
	}

	// Handle proto messages
	if protoMsg, ok := msg.(proto.Message); ok {
		marshaler := protojson.MarshalOptions{
			UseProtoNames: true,
		}
		data, err := marshaler.Marshal(protoMsg)
		if err != nil {
			return nil
		}

		var result map[string]interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			return nil
		}

		return result
	}

	// Fallback: try to marshal as JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}

	return result
}

// extractUserAgent extracts the user agent from context
func extractUserAgent(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	if values := md.Get("user-agent"); len(values) > 0 {
		return values[0]
	}

	return ""
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return uuid.New().String()
}

// NewStreamAuditInterceptor creates a new gRPC stream interceptor for audit logging
func NewStreamAuditInterceptor(cfg *InterceptorConfig) grpc.StreamServerInterceptor {
	if !cfg.Enabled || cfg.Repository == nil {
		// Return a no-op interceptor if disabled
		return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return handler(srv, ss)
		}
	}

	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// For now, just call the handler without logging
		// Stream logging would require more complex logic to capture multiple messages
		return handler(srv, ss)
	}
}
