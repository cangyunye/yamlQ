package conn

import (
	"context"
	"fmt"
	"time"

	"yamlq/db-gateway/dialect"
	"yamlq/db-gateway/errs"
)

type QueryResult struct {
	Columns   []string        `json:"columns"`
	Rows      [][]interface{} `json:"rows"`
	Truncated bool            `json:"truncated"`
	Error     *errs.Detail    `json:"error"`
	Duration  time.Duration   `json:"-"`
}

func (m *Manager) executeOne(mc *ManagedConn, task *QueryTask) *QueryResult {
	timeout := task.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	mc.LastActive = time.Now()
	start := time.Now()

	finalSQL := dialect.WrapPagination(mc.Driver, task.SQL, task.Page, task.PageSize)
	rows, err := mc.DB.QueryContext(ctx, finalSQL, task.Params...)
	if err != nil {
		return &QueryResult{
			Error:    errs.Classify(err, ctx),
			Duration: time.Since(start),
		}
	}
	defer rows.Close()

	columns, _ := rows.Columns()
	var resultRows [][]interface{}

	for rows.Next() {
		select {
		case <-ctx.Done():
			return &QueryResult{
				Columns:   columns,
				Rows:      resultRows,
				Truncated: true,
				Error: &errs.Detail{
					Code:    "QUERY_TIMEOUT",
					Message: fmt.Sprintf("query exceeded %v timeout, %d rows fetched", timeout, len(resultRows)),
					Hint:    "SQL may need optimization or increase timeout",
				},
				Duration: time.Since(start),
			}
		default:
		}

		values := make([]interface{}, len(columns))
		ptrs := make([]interface{}, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return &QueryResult{
				Columns:  columns,
				Rows:     resultRows,
				Error:    errs.Classify(err, ctx),
				Duration: time.Since(start),
			}
		}
		for i, v := range values {
			if b, ok := v.([]byte); ok {
				values[i] = string(b)
			}
		}

		if len(resultRows) >= m.cfg.MaxRowsPerQuery {
			return &QueryResult{
				Columns:   columns,
				Rows:      resultRows,
				Truncated: true,
				Duration:  time.Since(start),
			}
		}
		resultRows = append(resultRows, values)
	}

	return &QueryResult{
		Columns:  columns,
		Rows:     resultRows,
		Duration: time.Since(start),
	}
}
