package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
)

const cursorSortVersion = 1

type cursorPayload struct {
	Version   int       `json:"v"`
	TenantID  string    `json:"t"`
	Filter    string    `json:"f"`
	CreatedAt time.Time `json:"c"`
	ID        string    `json:"i"`
	ExpiresAt time.Time `json:"e"`
}

func (s *Service) GetTask(ctx context.Context, principal auth.Principal, query GetTask) (Envelope[TaskView], error) {
	var result Envelope[TaskView]
	if err := s.policy.Require(principal, "tasks:read"); err != nil {
		return result, err
	}
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		record, err := tx.GetTask(ctx, principal.TenantID, query.TaskID)
		if err != nil {
			return err
		}
		result = Envelope[TaskView]{Data: taskView(*record), Meta: Meta{ServerTime: now, ResourceVersion: record.StateVersion}}
		return nil
	})
	return result, err
}

func (s *Service) ListTasks(ctx context.Context, principal auth.Principal, query ListTasks) (Envelope[[]TaskView], error) {
	var result Envelope[[]TaskView]
	if err := s.policy.Require(principal, "tasks:read"); err != nil {
		return result, err
	}
	limit := query.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return result, invalid("limit")
	}
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		filter := filterDigest(query)
		listQuery := TaskListQuery{TenantID: principal.TenantID, Statuses: query.Statuses, PublisherAgentVersionID: query.PublisherAgentVersionID, Limit: limit + 1}
		if query.Cursor != "" {
			cursor, err := s.decodeCursor(query.Cursor, principal.TenantID, filter, now)
			if err != nil {
				return err
			}
			listQuery.AfterCreatedAt = cursor.CreatedAt
			listQuery.AfterID = cursor.ID
		}
		records, err := tx.ListTaskRecords(ctx, listQuery)
		if err != nil {
			return err
		}
		hasMore := len(records) > limit
		if hasMore {
			records = records[:limit]
		}
		views := make([]TaskView, len(records))
		for i := range records {
			views[i] = taskView(records[i])
		}
		result = Envelope[[]TaskView]{Data: views, Meta: Meta{ServerTime: now}}
		if hasMore {
			last := records[len(records)-1]
			result.Meta.NextCursor = s.encodeCursor(cursorPayload{Version: cursorSortVersion, TenantID: principal.TenantID, Filter: filter, CreatedAt: last.CreatedAt, ID: last.ID, ExpiresAt: now.Add(s.cursorTTL)})
		}
		return nil
	})
	return result, err
}

func taskView(record TaskRecord) TaskView {
	var constraints, requirements []string
	_ = json.Unmarshal(record.Constraints, &constraints)
	_ = json.Unmarshal(record.Requirements, &requirements)
	return TaskView{ID: record.ID, TenantID: record.TenantID, PublisherAgentVersionID: record.PublisherAgentVersionID, Type: record.Type, Title: record.Title, Problem: record.Problem, Constraints: constraints, Requirements: requirements, Deadline: record.Deadline, Status: record.Status, ClaimedBy: record.ClaimedBy, ActiveExecutionID: record.ActiveExecutionID, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, StateVersion: record.StateVersion}
}

func filterDigest(query ListTasks) string {
	statuses := append([]domain.TaskStatus(nil), query.Statuses...)
	sort.Slice(statuses, func(i, j int) bool { return statuses[i] < statuses[j] })
	parts := make([]string, len(statuses))
	for i := range statuses {
		parts[i] = string(statuses[i])
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, ",") + "\x00" + query.PublisherAgentVersionID))
	return hex.EncodeToString(sum[:])
}
func (s *Service) encodeCursor(payload cursorPayload) string {
	body, _ := json.Marshal(payload)
	mac := hmac.New(sha256.New, s.cursorSecret)
	_, _ = mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Service) decodeCursor(value, tenant, filter string, now time.Time) (cursorPayload, error) {
	var payload cursorPayload
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return payload, invalid("cursor")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return payload, invalid("cursor")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return payload, invalid("cursor")
	}
	mac := hmac.New(sha256.New, s.cursorSecret)
	_, _ = mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) || json.Unmarshal(body, &payload) != nil || payload.Version != cursorSortVersion || payload.TenantID != tenant || payload.Filter != filter || payload.ID == "" || !now.Before(payload.ExpiresAt) {
		return cursorPayload{}, invalid("cursor")
	}
	return payload, nil
}
