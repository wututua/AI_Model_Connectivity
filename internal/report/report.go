package report

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"cg/internal/config"
	"cg/internal/probe"
)

type HistoryRecord struct {
	Status    string `json:"status"`
	LatencyMS int    `json:"latency_ms"`
	CheckedAt string `json:"checked_at"`
}

type ModelResult struct {
	probe.Result
	History            []string `json:"history"`
	ShowCurveChart     bool     `json:"show_curve_chart"`
	IntervalStr        string   `json:"interval_str"`
	SVGPathLine        string   `json:"svg_path_line"`
	SVGPathArea        string   `json:"svg_path_area"`
	TimeLabels         []string `json:"time_labels"`
	AvgLatency24h      string   `json:"avg_latency_24h"`
	P50Latency24h      string   `json:"p50_latency_24h"`
	P95Latency24h      string   `json:"p95_latency_24h"`
	P99Latency24h      string   `json:"p99_latency_24h"`
	LatencySamples24h  int      `json:"latency_samples_24h"`
	WeeklySuccessCount int      `json:"weekly_success_count"`
	WeeklyTotalCount   int      `json:"weekly_total_count"`
	WeeklySuccessText  string   `json:"weekly_success_text"`
	Availability       string   `json:"availability"`
	StatusLabel        string   `json:"status_label"`
	StatusClass        string   `json:"status_class"`
}

type ProviderReport struct {
	ProviderID   string        `json:"provider_id"`
	ProviderType string        `json:"provider_type"`
	ProviderName string        `json:"provider_name"`
	ProviderLogo string        `json:"provider_logo"`
	CurrentModel string        `json:"current_model"`
	Results      []ModelResult `json:"results"`
	OKCount      int           `json:"ok_count"`
	SlowCount    int           `json:"slow_count"`
	ErrorCount   int           `json:"error_count"`
	Status       string        `json:"status"`
	StatusLabel  string        `json:"status_label"`
	ModelCount   int           `json:"model_count"`
}

type Report struct {
	Title               string                `json:"title"`
	GeneratedAt         string                `json:"generated_at"`
	ElapsedMS           int                   `json:"elapsed_ms"`
	GlobalConcurrency   int                   `json:"global_concurrency"`
	ProviderConcurrency int                   `json:"provider_concurrency"`
	Total               int                   `json:"total"`
	OKCount             int                   `json:"ok_count"`
	SlowCount           int                   `json:"slow_count"`
	ErrorCount          int                   `json:"error_count"`
	ProviderCount       int                   `json:"provider_count"`
	Providers           []ProviderReport      `json:"providers"`
	ProviderErrors      []probe.ProviderError `json:"provider_errors"`
	OverallStatus       string                `json:"overall_status"`
	OverallClass        string                `json:"overall_class"`
	HistorySize         int                   `json:"history_size"`
	StatsWindowDays     int                   `json:"stats_window_days"`
	Theme               string                `json:"theme"`
	ThemeLabel          string                `json:"theme_label"`
}

func Build(cfg config.Config, results []probe.Result, providerErrors []probe.ProviderError, history map[string][]HistoryRecord, started time.Time) (Report, map[string][]HistoryRecord) {
	now := time.Now()
	theme := themeName(cfg, now)
	interval := intervalLabel(cfg)
	updatedHistory := map[string][]HistoryRecord{}
	for key, records := range history {
		updatedHistory[key] = append([]HistoryRecord(nil), records...)
	}

	modelResults := make([]ModelResult, 0, len(results))
	for _, result := range results {
		records := pruneHistory(updatedHistory[result.HistoryKey], now, cfg.StatsWindowDays, cfg.HistorySize)
		records = append(records, HistoryRecord{Status: result.Status, LatencyMS: result.LatencyMS, CheckedAt: now.Format(time.RFC3339)})
		limit := max(cfg.HistorySize, cfg.MaxHistoryRecords)
		if len(records) > limit {
			records = records[len(records)-limit:]
		}
		updatedHistory[result.HistoryKey] = records

		model := ModelResult{Result: result}
		model.History = historyBars(records, cfg.HistorySize)
		model.ShowCurveChart = cfg.ShowCurveChart
		model.IntervalStr = interval
		if cfg.ShowCurveChart {
			latencies := historyLatencies(records, cfg.HistorySize)
			model.SVGPathLine = generateSVGPath(latencies, 100, 40)
			if model.SVGPathLine != "" {
				model.SVGPathArea = model.SVGPathLine + " L 100,40 L 0,40 Z"
			}
			model.TimeLabels = historyTimeLabels(records, cfg.HistorySize)
		}
		records24h := recordsInHours(records, now, 24)
		validLatencies := []int{}
		for _, record := range records24h {
			if record.Status == "ok" || record.Status == "slow" {
				validLatencies = append(validLatencies, record.LatencyMS)
			}
		}
		model.LatencySamples24h = len(validLatencies)
		if len(validLatencies) == 0 {
			model.AvgLatency24h = "N/A"
			model.P50Latency24h = "N/A"
			model.P95Latency24h = "N/A"
			model.P99Latency24h = "N/A"
		} else {
			sort.Ints(validLatencies)
			model.AvgLatency24h = fmt.Sprintf("%d ms", average(validLatencies))
			model.P50Latency24h = fmt.Sprintf("%d ms", percentile(validLatencies, 0.50))
			model.P95Latency24h = fmt.Sprintf("%d ms", percentile(validLatencies, 0.95))
			model.P99Latency24h = fmt.Sprintf("%d ms", percentile(validLatencies, 0.99))
		}
		windowRecords := recordsInDays(records, now, cfg.StatsWindowDays)
		model.WeeklySuccessCount, model.WeeklyTotalCount = successTotalCounts(windowRecords)
		model.WeeklySuccessText = fmt.Sprintf("%d/%d", model.WeeklySuccessCount, model.WeeklyTotalCount)
		model.Availability = availability(windowRecords)
		model.StatusLabel = statusLabel(model.Status)
		model.StatusClass = model.Status
		if !cfg.ShowErrorDetail {
			model.Error = ""
		}
		modelResults = append(modelResults, model)
	}

	grouped := map[string]*ProviderReport{}
	order := []string{}
	for _, result := range modelResults {
		group := grouped[result.ProviderID]
		if group == nil {
			group = &ProviderReport{ProviderID: result.ProviderID, ProviderType: result.ProviderType, ProviderName: result.ProviderName, ProviderLogo: result.ProviderLogo, CurrentModel: result.CurrentModel, Status: "ok", StatusLabel: "正常"}
			grouped[result.ProviderID] = group
			order = append(order, result.ProviderID)
		}
		group.Results = append(group.Results, result)
		switch result.Status {
		case "ok":
			group.OKCount++
		case "slow":
			group.SlowCount++
		case "error":
			group.ErrorCount++
		}
	}
	providers := []ProviderReport{}
	for _, key := range order {
		group := grouped[key]
		if group.ErrorCount > 0 {
			group.Status = "error"
			group.StatusLabel = "异常"
		} else if group.SlowCount > 0 {
			group.Status = "slow"
			group.StatusLabel = "较慢"
		}
		group.ModelCount = len(group.Results)
		providers = append(providers, *group)
	}

	okCount, slowCount, errorCount := 0, 0, 0
	for _, result := range results {
		switch result.Status {
		case "ok":
			okCount++
		case "slow":
			slowCount++
		case "error":
			errorCount++
		}
	}
	overallStatus := "OPERATIONAL"
	overallClass := "ok"
	if errorCount > 0 {
		overallStatus = "DEGRADED"
		overallClass = "error"
	}
	return Report{
		Title:               cfg.DashboardTitle,
		GeneratedAt:         now.Format("2006-01-02 15:04:05"),
		ElapsedMS:           int(time.Since(started).Milliseconds()),
		GlobalConcurrency:   cfg.Concurrency,
		ProviderConcurrency: cfg.ProviderConcurrency,
		Total:               len(results),
		OKCount:             okCount,
		SlowCount:           slowCount,
		ErrorCount:          errorCount,
		ProviderCount:       len(providers),
		Providers:           providers,
		ProviderErrors:      providerErrors,
		OverallStatus:       overallStatus,
		OverallClass:        overallClass,
		HistorySize:         cfg.HistorySize,
		StatsWindowDays:     cfg.StatsWindowDays,
		Theme:               theme,
		ThemeLabel:          map[bool]string{true: "白天", false: "夜间"}[theme == "light"],
	}, updatedHistory
}

func themeName(cfg config.Config, now time.Time) string {
	switch cfg.ThemeMode {
	case "light", "dark":
		return cfg.ThemeMode
	}
	hour := now.Hour()
	if cfg.DayModeStartHour <= cfg.DayModeEndHour {
		if hour >= cfg.DayModeStartHour && hour < cfg.DayModeEndHour {
			return "light"
		}
		return "dark"
	}
	if hour >= cfg.DayModeStartHour || hour < cfg.DayModeEndHour {
		return "light"
	}
	return "dark"
}

func intervalLabel(cfg config.Config) string {
	minHours := cfg.AutoCheckIntervalMinHours
	maxHours := cfg.AutoCheckIntervalMaxHours
	if minHours <= 0 && maxHours <= 0 {
		return "manual"
	}
	if minHours <= 0 {
		minHours = maxHours
	}
	if maxHours <= 0 {
		maxHours = minHours
	}
	if maxHours < minHours {
		minHours, maxHours = maxHours, minHours
	}
	if math.Abs(maxHours-minHours) < 0.0001 {
		return fmt.Sprintf("%gh", maxHours)
	}
	return fmt.Sprintf("%g-%gh", minHours, maxHours)
}

func historyBars(records []HistoryRecord, size int) []string {
	statuses := []string{}
	start := max(0, len(records)-size)
	for _, record := range records[start:] {
		statuses = append(statuses, record.Status)
	}
	padding := make([]string, max(0, size-len(statuses)))
	for i := range padding {
		padding[i] = "empty"
	}
	return append(padding, statuses...)
}

func historyLatencies(records []HistoryRecord, size int) []int {
	latencies := []int{}
	start := max(0, len(records)-size)
	for _, record := range records[start:] {
		latencies = append(latencies, record.LatencyMS)
	}
	padding := make([]int, max(0, size-len(latencies)))
	return append(padding, latencies...)
}

func generateSVGPath(latencies []int, width, height int) string {
	if len(latencies) == 0 {
		return ""
	}
	maxLat := 1000
	for _, latency := range latencies {
		if latency > maxLat {
			maxLat = latency
		}
	}
	if len(latencies) == 1 {
		y := float64(height) - float64(latencies[0])/float64(maxLat)*float64(height)
		return fmt.Sprintf("M 0,%.1f L %d,%.1f", y, width, y)
	}
	step := float64(width) / float64(len(latencies)-1)
	prevX, prevY := 0.0, float64(height)-float64(latencies[0])/float64(maxLat)*float64(height)
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("M %.1f,%.1f", prevX, prevY))
	for i := 1; i < len(latencies); i++ {
		x := float64(i) * step
		y := float64(height) - float64(latencies[i])/float64(maxLat)*float64(height)
		cx1 := prevX + step/2
		cx2 := x - step/2
		buf.WriteString(fmt.Sprintf(" C %.1f,%.1f %.1f,%.1f %.1f,%.1f", cx1, prevY, cx2, y, x, y))
		prevX, prevY = x, y
	}
	return buf.String()
}

func historyTimeLabels(records []HistoryRecord, size int) []string {
	labels := make([]string, size)
	start := max(0, len(records)-size)
	recent := records[start:]
	offset := size - len(recent)
	for i, record := range recent {
		checkedAt, err := time.Parse(time.RFC3339, record.CheckedAt)
		if err != nil {
			labels[offset+i] = ""
			continue
		}
		labels[offset+i] = checkedAt.Format("15:04")
	}
	return labels
}

func pruneHistory(records []HistoryRecord, now time.Time, statsDays, historySize int) []HistoryRecord {
	cutoff := now.Add(-time.Duration(max(1, statsDays)) * 24 * time.Hour)
	pruned := []HistoryRecord{}
	for _, record := range records {
		checkedAt, err := time.Parse(time.RFC3339, record.CheckedAt)
		if err == nil && checkedAt.Before(cutoff) {
			continue
		}
		pruned = append(pruned, record)
	}
	minimum := max(1, historySize)
	if len(pruned) < minimum && len(records) > len(pruned) {
		return records[max(0, len(records)-minimum):]
	}
	return pruned
}

func recordsInHours(records []HistoryRecord, now time.Time, hours int) []HistoryRecord {
	cutoff := now.Add(-time.Duration(hours) * time.Hour)
	return recordsSince(records, cutoff)
}

func recordsInDays(records []HistoryRecord, now time.Time, days int) []HistoryRecord {
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	return recordsSince(records, cutoff)
}

func recordsSince(records []HistoryRecord, cutoff time.Time) []HistoryRecord {
	result := []HistoryRecord{}
	for _, record := range records {
		checkedAt, err := time.Parse(time.RFC3339, record.CheckedAt)
		if err == nil && checkedAt.Before(cutoff) {
			continue
		}
		result = append(result, record)
	}
	return result
}

func successTotalCounts(records []HistoryRecord) (int, int) {
	success := 0
	for _, record := range records {
		if record.Status == "ok" || record.Status == "slow" {
			success++
		}
	}
	return success, len(records)
}

func availability(records []HistoryRecord) string {
	if len(records) == 0 {
		return "N/A"
	}
	success, total := successTotalCounts(records)
	return fmt.Sprintf("%.1f%%", float64(success)/float64(total)*100)
}

func average(values []int) int {
	sum := 0
	for _, value := range values {
		sum += value
	}
	return sum / len(values)
}

// percentile expects a pre-sorted ascending slice and returns the value at
// the given quantile p ∈ [0, 1] using Type-7 linear interpolation (numpy /
// Excel PERCENTILE default).  Returns 0 on empty input.
func percentile(sorted []int, p float64) int {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 || p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[n-1]
	}
	pos := p * float64(n-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return int(math.Round(float64(sorted[lo]) + frac*float64(sorted[hi]-sorted[lo])))
}

func statusLabel(status string) string {
	switch status {
	case "ok":
		return "正常"
	case "slow":
		return "较慢"
	case "error":
		return "错误"
	default:
		return "未知"
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
