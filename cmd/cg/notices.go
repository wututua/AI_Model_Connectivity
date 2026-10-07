package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"cg/internal/config"
	"cg/internal/notify"
)

func (a *application) sendOperationalNotices(ctx context.Context) {
	if !a.notificationMu.TryLock() {
		return
	}
	defer a.notificationMu.Unlock()
	settings, err := a.store.MonitoringSettings(ctx)
	if err != nil {
		return
	}
	cfg := a.currentConfig()
	if cfg.InMaintenance(time.Now()) {
		return
	}
	catalogs, err := a.store.CatalogEvents(ctx)
	if err != nil {
		return
	}
	events, err := a.store.MonitorEvents(ctx)
	if err != nil {
		return
	}
	cost, err := a.store.CostSummary(ctx, settings)
	if err != nil {
		return
	}
	type notice struct{ key, title, summary string }
	sent := 0
	for _, rule := range settings.Rules {
		if !rule.Enabled {
			continue
		}
		maintenance := false
		for _, schedule := range settings.Schedules {
			if schedule.ProviderID == rule.ProviderID && (config.OperationsSettings{MaintenanceStart: schedule.MaintenanceStart, MaintenanceEnd: schedule.MaintenanceEnd}).InMaintenance(time.Now()) {
				maintenance = true
			}
		}
		if maintenance {
			continue
		}
		pending := []notice{}
		if rule.CatalogChanges {
			for _, event := range catalogs {
				current := false
				for _, p := range cfg.Providers {
					if p.ID == event.ProviderID && p.ConnectionRevision == event.Revision && p.Enabled && p.ProbeEnabled {
						current = true
					}
				}
				if !current {
					continue
				}
				if rule.ProviderID != "" && event.ProviderID != rule.ProviderID {
					continue
				}
				if rule.Model != "" && !slices.Contains(event.Added, rule.Model) && !slices.Contains(event.Removed, rule.Model) {
					continue
				}
				created, err := time.Parse(time.RFC3339, event.CreatedAt)
				if err != nil || time.Since(created) > 24*time.Hour {
					continue
				}
				pending = append(pending, notice{"catalog:" + strconv.FormatInt(event.ID, 10), "模型清单变更", fmt.Sprintf("%s: 新增 %d，移除 %d；新增模型需在监控中心批准。", event.ProviderID, len(event.Added), len(event.Removed))})
			}
		}
		if rule.BackupFailures && rule.ProviderID == "" && rule.Model == "" {
			for _, event := range events {
				if event.Kind != "backup_error" {
					continue
				}
				created, err := time.Parse(time.RFC3339, event.CreatedAt)
				if err != nil || time.Since(created) > 24*time.Hour {
					continue
				}
				pending = append(pending, notice{"backup:" + strconv.FormatInt(event.ID, 10), "备份失败", "数据库备份未成功，请检查磁盘空间、权限和监控中心运行记录。"})
			}
		}
		if rule.BudgetAlerts && rule.ProviderID == "" && rule.Model == "" && cost.BudgetExceeded {
			pending = append(pending, notice{"budget:" + cost.Month, "月度费用预算提醒", fmt.Sprintf("%s 已知用量估算 $%.6f，提醒预算 $%.6f；未知费用请求 %d。估算不是账单。", cost.Month, cost.EstimatedUSD, cost.BudgetUSD, cost.UnknownProbes)})
		}
		for _, item := range pending {
			if sent >= 10 {
				return
			}
			currentSettings, err := a.store.MonitoringSettings(ctx)
			if err != nil || currentSettings.Version != settings.Version {
				return
			}
			key := rule.ID + ":" + item.key
			reserved, err := a.store.ReserveNotice(ctx, key, time.Now())
			if err != nil || !reserved {
				continue
			}
			sent++
			client := a.notificationClient(rule.Apply(cfg))
			client.SetRuleID(rule.ID)
			_, sendErr := client.SendOperational(ctx, item.title, item.summary)
			if sendErr == notify.ErrResultNotSaved {
				continue
			}
			_ = a.store.FinishNotice(ctx, key, sendErr != nil)
		}
	}
}
