package logquery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sls "github.com/aliyun/aliyun-log-go-sdk"
)

// slsConfigKeys are the expected keys in Config.Config for the "sls" backend.
const (
	slsKeyEndpoint        = "endpoint"
	slsKeyProject         = "project"
	slsKeyLogstore        = "logstore"
	slsKeyAccessKeyID     = "accessKeyId"
	slsKeyAccessKeySecret = "accessKeySecret"
)

type slsConfig struct {
	Endpoint        string
	Project         string
	Logstore        string
	AccessKeyID     string
	AccessKeySecret string
}

func parseSLSConfig(raw map[string]interface{}) (*slsConfig, error) {
	if raw == nil {
		return nil, fmt.Errorf("sls config is required")
	}
	// Round-trip through JSON to coerce map[string]interface{} into a typed
	// struct without hand-rolling per-field casts.
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal sls config: %w", err)
	}
	var decoded struct {
		Endpoint        string `json:"endpoint"`
		Project         string `json:"project"`
		Logstore        string `json:"logstore"`
		AccessKeyID     string `json:"accessKeyId"`
		AccessKeySecret string `json:"accessKeySecret"`
	}
	if err := json.Unmarshal(buf, &decoded); err != nil {
		return nil, fmt.Errorf("decode sls config: %w", err)
	}
	cfg := &slsConfig{
		Endpoint:        strings.TrimSpace(decoded.Endpoint),
		Project:         strings.TrimSpace(decoded.Project),
		Logstore:        strings.TrimSpace(decoded.Logstore),
		AccessKeyID:     strings.TrimSpace(decoded.AccessKeyID),
		AccessKeySecret: strings.TrimSpace(decoded.AccessKeySecret),
	}
	if err := validateSLSConfigStruct(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validateSLSConfigStruct(cfg *slsConfig) error {
	if cfg.Endpoint == "" {
		return fmt.Errorf("sls %s is required", slsKeyEndpoint)
	}
	if cfg.Project == "" {
		return fmt.Errorf("sls %s is required", slsKeyProject)
	}
	if cfg.Logstore == "" {
		return fmt.Errorf("sls %s is required", slsKeyLogstore)
	}
	if cfg.AccessKeyID == "" {
		return fmt.Errorf("sls %s is required", slsKeyAccessKeyID)
	}
	if cfg.AccessKeySecret == "" {
		return fmt.Errorf("sls %s is required", slsKeyAccessKeySecret)
	}
	return nil
}

func validateSLSConfig(raw map[string]interface{}) error {
	_, err := parseSLSConfig(raw)
	return err
}

// slsQuerier implements Querier against Aliyun SLS.
type slsQuerier struct {
	client   sls.ClientInterface
	project  string
	logstore string
}

func newSLSQuerier(raw map[string]interface{}) (Querier, error) {
	cfg, err := parseSLSConfig(raw)
	if err != nil {
		return nil, err
	}
	client := sls.CreateNormalInterface(cfg.Endpoint, cfg.AccessKeyID, cfg.AccessKeySecret, "")
	return &slsQuerier{
		client:   client,
		project:  cfg.Project,
		logstore: cfg.Logstore,
	}, nil
}

// buildQuery combines the user-supplied Raw query with the platform-injected
// Labels using SLS's `and` conjunction. Label values are double-quoted and
// escaped to avoid breaking the query syntax.
func (q *slsQuerier) buildQuery(query Query) string {
	var parts []string
	for k, v := range query.Labels {
		if k == "" || v == "" {
			continue
		}
		escaped := strings.ReplaceAll(v, `"`, `\"`)
		// Add "content." prefix for JSON nested fields in SLS
		parts = append(parts, fmt.Sprintf(`content.%s:"%s"`, k, escaped))
	}
	raw := strings.TrimSpace(query.Raw)
	if raw != "" {
		parts = append(parts, "("+raw+")")
	}
	if len(parts) == 0 {
		// "*" matches everything in SLS syntax.
		return "*"
	}
	return strings.Join(parts, " and ")
}

func (q *slsQuerier) Query(ctx context.Context, query Query) (*Result, error) {
	limit := int64(query.Limit)
	// 为了精准判断 HasMore，向底层请求 limit + 1 条数据。 如果返回了 limit + 1 条，说明肯定还有下一页
	fetchLimit := limit + 1
	queryExp := q.buildQuery(query)
	toUnix := query.To.Unix()
	var offset int64
	if query.Cursor != "" {
		if _, err := fmt.Sscanf(query.Cursor, "%d:%d", &toUnix, &offset); err != nil {
			return nil, fmt.Errorf("invalid cursor %q: %w", query.Cursor, err)
		}
	}

	logs, err := q.client.GetLogs(
		q.project,
		q.logstore,
		"",
		query.From.Unix(),
		toUnix,
		queryExp,
		fetchLimit,
		offset,
		query.Reverse,
	)
	if err != nil {
		return nil, fmt.Errorf("sls GetLogs: %w", err)
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var parsed struct {
		Content   string `json:"content"`
		Task      string `json:"task"`
		Pod       string `json:"pod"`
		Namespace string `json:"namespace"`
		Node      string `json:"node"`
		Container string `json:"container"`
		Stream    string `json:"stream"`
		Time      string `json:"time"`
	}

	result := &Result{}
	for _, lg := range logs.Logs {
		entry := Entry{
			Fields: map[string]interface{}{},
			Labels: map[string]string{},
		}
		for k, v := range lg {
			switch k {
			case "__time__":
				if t, err := json.Number(v).Int64(); err == nil {
					entry.Timestamp = time.Unix(t, 0)
				}
			case "content", "log", "__tag__:__raw_log__":
				entry.Line = v
			default:
				entry.Fields[k] = v
			}
		}

		if err := json.Unmarshal([]byte(entry.Line), &parsed); err == nil {
			if parsed.Content != "" {
				entry.Line = parsed.Content
			}
			if parsed.Task != "" {
				entry.Labels["task"] = parsed.Task
			}
			if parsed.Pod != "" {
				entry.Labels["pod"] = parsed.Pod
			}
			if parsed.Namespace != "" {
				entry.Labels["namespace"] = parsed.Namespace
			}
			if parsed.Node != "" {
				entry.Labels["node"] = parsed.Node
			}
			if parsed.Container != "" {
				entry.Labels["container"] = parsed.Container
			}
			if parsed.Stream != "" {
				entry.Labels["stream"] = parsed.Stream
			}
			if parsed.Time != "" {
				if t, err := time.Parse(time.RFC3339Nano, parsed.Time); err == nil {
					entry.Timestamp = t
				}
			}
		}

		result.Entries = append(result.Entries, entry)
	}

	// 如果拿到的数据超过了 limit，说明有下一页。截断多余的探针数据。
	if int64(len(result.Entries)) > limit {
		result.HasMore = true
		result.Entries = result.Entries[:limit]
		result.NextCursor = fmt.Sprintf("%d:%d", toUnix, offset+limit)
	} else {
		result.HasMore = false
	}

	return result, nil
}

// LabelValues implements Querier.LabelValues for Aliyun SLS.
// It uses SLS SQL analysis (SELECT DISTINCT) to get unique values efficiently,
// avoiding the 100-line limit of raw log queries.
// NOTE: Requires the target field to have analytics enabled in SLS index config.
func (q *slsQuerier) LabelValues(ctx context.Context, label string, from, to time.Time, filters map[string]string) ([]string, error) {
	// 构造过滤条件（WHERE 部分）
	var conditions []string
	for k, v := range filters {
		if k == "" || v == "" {
			continue
		}
		escaped := strings.ReplaceAll(v, `'`, `\'`)
		conditions = append(conditions, fmt.Sprintf(`"content.%s" = '%s'`, k, escaped))
	}
	// 确保目标字段存在（非空）
	conditions = append(conditions, fmt.Sprintf(`"content.%s" IS NOT NULL`, label))
	whereClause := strings.Join(conditions, " AND ")

	// 使用 SLS SQL 分析查询 DISTINCT 值
	// 语法：* | SELECT DISTINCT "content.pod" AS value WHERE ... LIMIT 1000
	queryExp := fmt.Sprintf(`* | SELECT DISTINCT "content.%s" AS value LIMIT 1000`, label)
	if whereClause != "" {
		queryExp = fmt.Sprintf(`* | SELECT DISTINCT "content.%s" AS value WHERE %s LIMIT 1000`, label, whereClause)
	}

	logs, err := q.client.GetLogs(
		q.project,
		q.logstore,
		"",
		from.Unix(),
		to.Unix(),
		queryExp,
		1000,
		0,
		false,
	)
	if err != nil {
		return nil, fmt.Errorf("sls GetLogs for label values: %w", err)
	}

	valueSet := make(map[string]struct{})
	for _, lg := range logs.Logs {
		for k, v := range lg {
			// SQL 查询返回的字段名是 "value"（我们在 SELECT 中起的别名）
			if k == "value" && v != "" {
				valueSet[v] = struct{}{}
			}
		}
	}

	values := make([]string, 0, len(valueSet))
	for v := range valueSet {
		values = append(values, v)
	}

	return values, nil
}
