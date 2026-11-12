package datachangelog

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
)

var identRe = regexp.MustCompile(`^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)?$`)

// ClickHouseRepository mengimplementasikan Repository dengan ClickHouse sebagai storage.
type ClickHouseRepository struct {
	conn  chdriver.Conn
	cfg   *ClickHouseConfig
	table string
}

func NewClickHouseRepository(cfg *ClickHouseConfig) (*ClickHouseRepository, error) {
	if cfg == nil {
		return nil, fmt.Errorf("clickhouse config cannot be nil")
	}
	if cfg.Addr == "" {
		return nil, fmt.Errorf("clickhouse addr must be specified")
	}
	table := cfg.Table
	if table == "" {
		table = "audit_logs"
	}
	if !identRe.MatchString(table) {
		return nil, fmt.Errorf("invalid clickhouse table name %q", table)
	}

	opts := &clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		DialTimeout: 10 * time.Second,
	}
	if cfg.TLS {
		opts.TLS = &tls.Config{}
	}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open clickhouse connection: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to ping clickhouse: %w", err)
	}

	return &ClickHouseRepository{conn: conn, cfg: cfg, table: table}, nil
}

func (r *ClickHouseRepository) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if r.cfg.RequestTimeout > 0 {
		return context.WithTimeout(ctx, r.cfg.RequestTimeout)
	}
	return ctx, func() {}
}

// ---------- Write ----------

func (r *ClickHouseRepository) Save(ctx context.Context, l *DataChangeLog) error {
	if l == nil {
		return fmt.Errorf("log cannot be nil")
	}
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return r.insert(ctx, []DataChangeLog{*l})
}

func (r *ClickHouseRepository) SaveBatch(ctx context.Context, logs []DataChangeLog) error {
	if len(logs) == 0 {
		return nil
	}
	for i := range logs {
		if logs[i].ID == "" {
			logs[i].ID = uuid.New().String()
		}
	}
	return r.insert(ctx, logs)
}

func (r *ClickHouseRepository) insert(ctx context.Context, logs []DataChangeLog) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	if r.cfg.AsyncInsert {
		wait := 0
		if r.cfg.WaitForAsyncInsert {
			wait = 1
		}
		ctx = clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
			"async_insert":          1,
			"wait_for_async_insert": wait,
		}))
	}

	batch, err := r.conn.PrepareBatch(ctx, fmt.Sprintf(
		`INSERT INTO %s (id, domain, entity, operation, primary_key, primary_key_str,
			change_data, after_data, changed_by, changed_by_email, change_timestamp,
			request_id, ip_address, user_agent, metadata)`, r.table))
	if err != nil {
		return fmt.Errorf("clickhouse prepare batch: %w", err)
	}

	for i := range logs {
		l := &logs[i]
		ts := l.ChangeTimestamp
		if ts.IsZero() {
			ts = time.Now()
		}
		if err := batch.Append(
			l.ID, l.Domain, l.Entity, l.Operation,
			toJSONString(l.PrimaryKey), l.PrimaryKeyStr,
			toJSONString(l.ChangeData), toJSONString(l.AfterData),
			l.ChangedBy, l.ChangedByEmail, ts.UTC(),
			l.RequestID, l.IPAddress, l.UserAgent,
			toJSONString(l.Metadata),
		); err != nil {
			_ = batch.Abort()
			return fmt.Errorf("clickhouse append: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("clickhouse send: %w", err)
	}
	return nil
}

// ---------- Read ----------

const selectCols = `id, domain, entity, operation, primary_key, primary_key_str,
	change_data, after_data, changed_by, changed_by_email, change_timestamp,
	request_id, ip_address, user_agent, metadata`

func (r *ClickHouseRepository) Query(ctx context.Context, q *ChangeLogQuery) (*ChangeLogQueryResult, error) {
	if q == nil {
		q = &ChangeLogQuery{}
	}
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	where, args := buildWhere(q.Domain, q.Entity, q.PrimaryKeyStr, q.Operation, q.ChangedBy, q.StartDate, q.EndDate)

	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 10000 {
		limit = 10000
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	var total uint64
	if err := r.conn.QueryRow(ctx,
		fmt.Sprintf("SELECT count() FROM %s%s", r.table, where), args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("clickhouse count: %w", err)
	}

	rows, err := r.conn.Query(ctx, fmt.Sprintf(
		"SELECT %s FROM %s%s ORDER BY change_timestamp DESC LIMIT %d OFFSET %d",
		selectCols, r.table, where, limit, offset), args...)
	if err != nil {
		return nil, fmt.Errorf("clickhouse query: %w", err)
	}
	defer rows.Close()

	records := make([]DataChangeLog, 0)
	for rows.Next() {
		var (
			l                                   DataChangeLog
			pk, changeData, afterData, metadata string
			ts                                  time.Time
		)
		if err := rows.Scan(&l.ID, &l.Domain, &l.Entity, &l.Operation, &pk, &l.PrimaryKeyStr,
			&changeData, &afterData, &l.ChangedBy, &l.ChangedByEmail, &ts,
			&l.RequestID, &l.IPAddress, &l.UserAgent, &metadata); err != nil {
			return nil, fmt.Errorf("clickhouse scan: %w", err)
		}
		l.PrimaryKey = fromJSONString(pk)
		l.ChangeData = fromJSONString(changeData)
		l.AfterData = fromJSONString(afterData)
		l.Metadata = fromJSONString(metadata)
		l.ChangeTimestamp = ts.UTC()
		records = append(records, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("clickhouse rows: %w", err)
	}

	return &ChangeLogQueryResult{Total: int64(total), Limit: limit, Offset: offset, Records: records}, nil
}

func (r *ClickHouseRepository) GetByPrimaryKey(ctx context.Context, domain, entity, primaryKey string, limit, offset int) (*ChangeLogQueryResult, error) {
	return r.Query(ctx, &ChangeLogQuery{
		Domain: domain, Entity: entity, PrimaryKeyStr: primaryKey, Limit: limit, Offset: offset,
	})
}

func (r *ClickHouseRepository) GetEntityHistory(ctx context.Context, domain, entity, primaryKey string) (*EntityChangeHistory, error) {
	res, err := r.Query(ctx, &ChangeLogQuery{
		Domain: domain, Entity: entity, PrimaryKeyStr: primaryKey, Limit: 10000,
	})
	if err != nil {
		return nil, err
	}

	h := &EntityChangeHistory{
		Domain:        domain,
		Entity:        entity,
		PrimaryKeyStr: primaryKey,
		Changes:       res.Records,
		ChangedByList: []string{},
		Operations:    make(map[string]int64),
	}
	users := make(map[string]bool)
	for _, l := range res.Records {
		h.ChangeCount++
		if l.ChangedBy != "" && !users[l.ChangedBy] {
			users[l.ChangedBy] = true
			h.ChangedByList = append(h.ChangedByList, l.ChangedBy)
		}
		h.Operations[l.Operation]++
		if h.FirstChange.IsZero() || l.ChangeTimestamp.Before(h.FirstChange) {
			h.FirstChange = l.ChangeTimestamp
		}
		if l.ChangeTimestamp.After(h.LastChange) {
			h.LastChange = l.ChangeTimestamp
		}
	}
	return h, nil
}

func (r *ClickHouseRepository) GetStats(ctx context.Context, domain, entity string, startDate, endDate time.Time) (*AuditStats, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	stats := &AuditStats{
		Domain:          domain,
		Entity:          entity,
		OperationCounts: make(map[string]int64),
		DateRange:       DateRange{Start: startDate, End: endDate},
	}
	where, args := buildWhere(domain, entity, "", "", "", startDate, endDate)

	var total, users, entities uint64
	if err := r.conn.QueryRow(ctx, fmt.Sprintf(
		"SELECT count(), uniqExact(changed_by), uniqExact(primary_key_str) FROM %s%s", r.table, where),
		args...).Scan(&total, &users, &entities); err != nil {
		return nil, fmt.Errorf("clickhouse stats: %w", err)
	}
	stats.TotalRecords = int64(total)
	stats.UniqueUsers = int64(users)
	stats.UniqueEntities = int64(entities)

	rows, err := r.conn.Query(ctx, fmt.Sprintf(
		"SELECT operation, count() FROM %s%s GROUP BY operation", r.table, where), args...)
	if err != nil {
		return nil, fmt.Errorf("clickhouse stats operations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var op string
		var cnt uint64
		if err := rows.Scan(&op, &cnt); err != nil {
			return nil, fmt.Errorf("clickhouse stats scan: %w", err)
		}
		stats.OperationCounts[op] = int64(cnt)
	}
	return stats, rows.Err()
}

// DeleteOlderThan memakai mutation (ALTER TABLE ... DELETE) yang berjalan async dan berat.
// Untuk retensi, lebih baik pakai TTL di DDL tabel.
func (r *ClickHouseRepository) DeleteOlderThan(ctx context.Context, domain, entity string, date time.Time) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	where, args := buildWhere(domain, entity, "", "", "", time.Time{}, time.Time{})
	cond := "change_timestamp < ?"
	args = append(args, date.UTC())
	if where == "" {
		where = " WHERE " + cond
	} else {
		where += " AND " + cond
	}
	if err := r.conn.Exec(ctx, fmt.Sprintf("ALTER TABLE %s DELETE%s", r.table, where), args...); err != nil {
		return fmt.Errorf("clickhouse delete: %w", err)
	}
	return nil
}

func (r *ClickHouseRepository) Close() error { return r.conn.Close() }

func (r *ClickHouseRepository) Health(ctx context.Context) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()
	if err := r.conn.Ping(ctx); err != nil {
		return fmt.Errorf("clickhouse health check failed: %w", err)
	}
	return nil
}

// ---------- Helpers ----------

func buildWhere(domain, entity, pkStr, operation, changedBy string, start, end time.Time) (string, []any) {
	var conds []string
	var args []any
	add := func(cond string, v any) {
		conds = append(conds, cond)
		args = append(args, v)
	}
	if domain != "" {
		add("domain = ?", domain)
	}
	if entity != "" {
		add("entity = ?", entity)
	}
	if pkStr != "" {
		add("primary_key_str = ?", pkStr)
	}
	if operation != "" {
		add("operation = ?", operation)
	}
	if changedBy != "" {
		add("changed_by = ?", changedBy)
	}
	if !start.IsZero() {
		add("change_timestamp >= ?", start.UTC())
	}
	if !end.IsZero() {
		add("change_timestamp <= ?", end.UTC())
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func toJSONString(m map[string]interface{}) string {
	if m == nil {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func fromJSONString(s string) map[string]interface{} {
	out := map[string]interface{}{}
	if s == "" {
		return out
	}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

// ---------- Dual write (masa transisi) ----------

// DualWriteRepository menulis ke Primary (ES) dan Secondary (ClickHouse).
// Pembacaan hanya dari Primary. Kegagalan Secondary hanya di-log, tidak
// menggagalkan request, sehingga ES tetap jadi sumber kebenaran selama uji.
type DualWriteRepository struct {
	Primary   Repository
	Secondary Repository
}

func (d *DualWriteRepository) Save(ctx context.Context, l *DataChangeLog) error {
	err := d.Primary.Save(ctx, l) // Primary mengisi l.ID bila kosong
	if serr := d.Secondary.Save(ctx, l); serr != nil {
		log.Printf("[AUDIT] secondary (clickhouse) save failed: %v", serr)
	}
	return err
}

func (d *DualWriteRepository) SaveBatch(ctx context.Context, logs []DataChangeLog) error {
	err := d.Primary.SaveBatch(ctx, logs)
	if serr := d.Secondary.SaveBatch(ctx, logs); serr != nil {
		log.Printf("[AUDIT] secondary (clickhouse) save batch failed: %v", serr)
	}
	return err
}

func (d *DualWriteRepository) Query(ctx context.Context, q *ChangeLogQuery) (*ChangeLogQueryResult, error) {
	return d.Primary.Query(ctx, q)
}

func (d *DualWriteRepository) GetByPrimaryKey(ctx context.Context, domain, entity, primaryKey string, limit, offset int) (*ChangeLogQueryResult, error) {
	return d.Primary.GetByPrimaryKey(ctx, domain, entity, primaryKey, limit, offset)
}

func (d *DualWriteRepository) GetEntityHistory(ctx context.Context, domain, entity, primaryKey string) (*EntityChangeHistory, error) {
	return d.Primary.GetEntityHistory(ctx, domain, entity, primaryKey)
}

func (d *DualWriteRepository) DeleteOlderThan(ctx context.Context, domain, entity string, date time.Time) error {
	err := d.Primary.DeleteOlderThan(ctx, domain, entity, date)
	if serr := d.Secondary.DeleteOlderThan(ctx, domain, entity, date); serr != nil {
		log.Printf("[AUDIT] secondary (clickhouse) delete failed: %v", serr)
	}
	return err
}

func (d *DualWriteRepository) GetStats(ctx context.Context, domain, entity string, startDate, endDate time.Time) (*AuditStats, error) {
	return d.Primary.GetStats(ctx, domain, entity, startDate, endDate)
}

func (d *DualWriteRepository) Close() error {
	err := d.Primary.Close()
	if serr := d.Secondary.Close(); serr != nil && err == nil {
		err = serr
	}
	return err
}

func (d *DualWriteRepository) Health(ctx context.Context) error {
	err := d.Primary.Health(ctx)
	if serr := d.Secondary.Health(ctx); serr != nil {
		log.Printf("[AUDIT] secondary (clickhouse) health check failed: %v", serr)
	}
	return err
}
