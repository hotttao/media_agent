package platform_administration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

var decimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,29})(\.[0-9]{1,18})?$`)

type PlatformStatistic struct {
	MetricKey, DimensionsJSON, DimensionsHash, Value string
	PeriodStart, RefreshedAt                         time.Time
}

type StatisticFilter struct {
	MetricKey  string
	PeriodFrom time.Time
	PeriodTo   time.Time
}

type StatisticsRepository interface {
	UpsertStatistic(context.Context, *PlatformStatistic) error
	GetStatistic(context.Context, string, time.Time, string) (*PlatformStatistic, error)
	ListStatistics(context.Context, StatisticFilter) ([]PlatformStatistic, error)
}

func ValidateStatistic(metricKey, dimensionsJSON, value string, periodStart, refreshedAt time.Time) (*PlatformStatistic, error) {
	metricKey, dimensionsJSON, value = strings.TrimSpace(metricKey), strings.TrimSpace(dimensionsJSON), strings.TrimSpace(value)
	if !settingKeyPattern.MatchString(metricKey) {
		return nil, errors.New("metric_key is invalid")
	}
	if periodStart.IsZero() || refreshedAt.IsZero() {
		return nil, errors.New("period_start and refreshed_at must be positive")
	}
	if dimensionsJSON == "" || len(dimensionsJSON) > 64*1024 {
		return nil, errors.New("dimensions_json must be a JSON object no larger than 64 KiB")
	}
	var dimensions map[string]any
	if err := json.Unmarshal([]byte(dimensionsJSON), &dimensions); err != nil || dimensions == nil {
		return nil, errors.New("dimensions_json must be a JSON object no larger than 64 KiB")
	}
	canonical, err := json.Marshal(dimensions)
	if err != nil {
		return nil, err
	}
	if !decimalPattern.MatchString(value) {
		return nil, errors.New("value must be a decimal with at most 30 integer and 18 fractional digits")
	}
	hash := sha256.Sum256(canonical)
	return &PlatformStatistic{MetricKey: metricKey, PeriodStart: periodStart, DimensionsJSON: string(canonical), DimensionsHash: hex.EncodeToString(hash[:]), Value: value, RefreshedAt: refreshedAt}, nil
}

func ValidateStatisticFilter(metricKey string, fromMillis, toMillis int64) (StatisticFilter, error) {
	metricKey = strings.TrimSpace(metricKey)
	if metricKey != "" && !settingKeyPattern.MatchString(metricKey) {
		return StatisticFilter{}, errors.New("metric_key is invalid")
	}
	filter := StatisticFilter{MetricKey: metricKey}
	if fromMillis > 0 {
		filter.PeriodFrom = time.UnixMilli(fromMillis).UTC()
	}
	if toMillis > 0 {
		filter.PeriodTo = time.UnixMilli(toMillis).UTC()
	}
	if !filter.PeriodFrom.IsZero() && !filter.PeriodTo.IsZero() && filter.PeriodFrom.After(filter.PeriodTo) {
		return StatisticFilter{}, errors.New("period_start_from must not exceed period_start_to")
	}
	return filter, nil
}
