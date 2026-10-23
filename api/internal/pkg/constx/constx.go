package constx

const (
	TableCreateTypeExist              int = 1
	TableCreateTypeJSONEachRow        int = 2
	TableCreateTypeJSONAsString       int = 6
	TableCreateTypeTraceCalculation   int = 4
	TableCreateTypeBufferNullDataPipe int = 5

	// Deprecated: TableCreateTypeCV
	TableCreateTypeCV int = 0
	// Deprecated: TableCreateTypeUBW
	TableCreateTypeUBW int = 3
)

const (
	UBWKafkaStreamField = "body"
)

// IsKafkaPipelineTable reports whether ClickVisual feeds the log store through a
// Kafka stream table plus materialized view - the only shape that can be
// suspended. Self-built(1), trace-calculation(4) and buffer-null(5) tables have
// no stream table to detach. Same set as RebuildStorageInt64 accepts.
func IsKafkaPipelineTable(createType int) bool {
	switch createType {
	case TableCreateTypeCV, TableCreateTypeJSONEachRow, TableCreateTypeUBW, TableCreateTypeJSONAsString:
		return true
	}
	return false
}

var (
	DefaultFields = map[string]interface{}{
		"_raw_log_":         struct{}{},
		"_time_second_":     struct{}{},
		"_time_nanosecond_": struct{}{},

		"_key":           struct{}{},
		"_headers.name":  struct{}{},
		"_headers.value": struct{}{},
	}
)
