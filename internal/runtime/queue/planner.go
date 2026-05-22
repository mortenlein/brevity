package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const PlanSchema = "brevity.runtime-queue-plan.v1"

type Plan struct {
	Schema           string      `json:"schema"`
	Path             string      `json:"path"`
	State            string      `json:"state"`
	Version          int         `json:"version"`
	SupportedVersion int         `json:"supportedVersion"`
	Runnable         []PlanItem  `json:"runnable"`
	Skipped          []PlanItem  `json:"skipped"`
	Summary          PlanSummary `json:"summary"`
	Error            string      `json:"error,omitempty"`
	ReadOnly         bool        `json:"readOnly"`
}

type PlanItem struct {
	ID       string `json:"id"`
	Task     string `json:"task"`
	Provider string `json:"provider,omitempty"`
	Profile  string `json:"profile,omitempty"`
	Status   string `json:"status,omitempty"`
	Reason   string `json:"reason"`
}

type PlanSummary struct {
	Runnable int `json:"runnable"`
	Skipped  int `json:"skipped"`
	Reserved int `json:"reserved"`
}

func (store Store) Plan() Plan {
	plan := Plan{
		Schema:           PlanSchema,
		Path:             store.QueuePath(),
		State:            "missing",
		Version:          Version,
		SupportedVersion: Version,
		Runnable:         []PlanItem{},
		Skipped:          []PlanItem{},
		ReadOnly:         true,
	}
	data, err := os.ReadFile(filepath.Clean(store.QueuePath()))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return finalizePlan(plan)
		}
		plan.State = "invalid"
		plan.Error = fmt.Sprintf("read %s: %v", FileName, err)
		return finalizePlan(plan)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		plan.State = "corrupted"
		plan.Error = fmt.Sprintf("read %s: file is empty", FileName)
		return finalizePlan(plan)
	}
	var queue Queue
	if err := json.Unmarshal(data, &queue); err != nil {
		plan.State = "corrupted"
		plan.Error = fmt.Sprintf("parse %s: %v", FileName, err)
		return finalizePlan(plan)
	}
	plan.State = "valid"
	plan.Version = queue.Version
	if queue.Version != Version {
		plan.State = "invalid"
		if queue.Version > Version {
			plan.Error = fmt.Sprintf("unsupported future runtime-queue.json version %d; supported version is %d", queue.Version, Version)
		} else {
			plan.Error = fmt.Sprintf("unsupported runtime-queue.json version %d; supported version is %d", queue.Version, Version)
		}
		return finalizePlan(plan)
	}

	duplicateIDs := duplicateItemIDs(queue.Items)
	for index, item := range queue.Items {
		reason := planningSkipReason(index, item, duplicateIDs)
		if reason != "" {
			plan.Skipped = append(plan.Skipped, planItem(item, reason))
			continue
		}
		if item.Reservation != nil {
			owner := strings.TrimSpace(item.Reservation.Owner)
			if owner == "" {
				owner = "(unknown)"
			}
			plan.Skipped = append(plan.Skipped, planItem(item, "reserved by "+owner))
			continue
		}
		plan.Runnable = append(plan.Runnable, planItem(item, "queued"))
	}
	return finalizePlan(plan)
}

func duplicateItemIDs(items []Item) map[string]struct{} {
	seen := map[string]struct{}{}
	duplicates := map[string]struct{}{}
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			duplicates[id] = struct{}{}
			continue
		}
		seen[id] = struct{}{}
	}
	return duplicates
}

func planningSkipReason(index int, item Item, duplicateIDs map[string]struct{}) string {
	id := strings.TrimSpace(item.ID)
	if id == "" {
		return fmt.Sprintf("item[%d] id is required", index)
	}
	if _, duplicate := duplicateIDs[id]; duplicate {
		return "duplicate queue id"
	}
	if _, err := NormalizeTaskSlug(item.Task); err != nil {
		return "invalid task slug"
	}
	status := strings.ToLower(strings.TrimSpace(item.Status))
	if status != StatusQueued {
		if status == "" {
			return "unsupported status: (missing)"
		}
		return fmt.Sprintf("unsupported status: %s", item.Status)
	}
	if _, err := parseTime(item.CreatedAt); err != nil {
		return fmt.Sprintf("invalid createdAt: %v", err)
	}
	if _, err := parseTime(item.UpdatedAt); err != nil {
		return fmt.Sprintf("invalid updatedAt: %v", err)
	}
	if item.Reservation != nil {
		if err := validateReservation(*item.Reservation); err != nil {
			return fmt.Sprintf("invalid reservation: %v", err)
		}
	}
	return ""
}

func planItem(item Item, reason string) PlanItem {
	return PlanItem{
		ID:       strings.TrimSpace(item.ID),
		Task:     strings.TrimSpace(item.Task),
		Provider: strings.TrimSpace(item.Provider),
		Profile:  strings.TrimSpace(item.Profile),
		Status:   strings.TrimSpace(item.Status),
		Reason:   reason,
	}
}

func finalizePlan(plan Plan) Plan {
	reserved := 0
	for _, item := range plan.Skipped {
		if strings.HasPrefix(item.Reason, "reserved by ") {
			reserved++
		}
	}
	plan.Summary = PlanSummary{
		Runnable: len(plan.Runnable),
		Skipped:  len(plan.Skipped),
		Reserved: reserved,
	}
	return plan
}
