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

const defaultPollAfterSeconds = 30

func (s *Service) GetTask(ctx context.Context, principal auth.Principal, query GetTask) (Envelope[TaskView], error) {
	var result Envelope[TaskView]
	if err := s.policy.Require(principal, "tasks:read"); err != nil {
		return result, err
	}
	err := s.store.WithTx(ctx, func(tx Tx) error {
		if err := s.requireLiveAgent(ctx, tx, principal); err != nil {
			return err
		}
		if err := s.checkRateLimit(ctx, principal); err != nil {
			return err
		}
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		record, err := tx.GetTask(ctx, principal.TenantID, query.TaskID)
		if err != nil {
			return err
		}
		view, err := taskView(*record)
		if err != nil {
			return err
		}
		views := []TaskView{view}
		if err := s.attachTaskSources(ctx, principal.TenantID, views); err != nil {
			return err
		}
		view = views[0]
		result = Envelope[TaskView]{Data: view, Meta: Meta{ServerTime: now, ResourceVersion: record.StateVersion, PollAfterSeconds: defaultPollAfterSeconds}}
		return nil
	})
	return result, err
}

func (s *Service) ListTasks(ctx context.Context, principal auth.Principal, query ListTasks) (Envelope[TaskPage], error) {
	var result Envelope[TaskPage]
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
		if err := s.requireLiveAgent(ctx, tx, principal); err != nil {
			return err
		}
		if err := s.checkRateLimit(ctx, principal); err != nil {
			return err
		}
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		filter := filterDigest(query)
		listQuery := TaskListQuery{TenantID: principal.TenantID, Statuses: query.Statuses, Type: query.Type, PublisherAgentVersionID: query.PublisherAgentVersionID, Limit: limit + 1}
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
			views[i], err = taskView(records[i])
			if err != nil {
				return err
			}
		}
		if err := s.attachTaskSources(ctx, principal.TenantID, views); err != nil {
			return err
		}
		result = Envelope[TaskPage]{Data: TaskPage{Items: views}, Meta: Meta{ServerTime: now, PollAfterSeconds: defaultPollAfterSeconds}}
		if hasMore {
			last := records[len(records)-1]
			result.Meta.NextCursor = s.encodeCursor(cursorPayload{Version: cursorSortVersion, TenantID: principal.TenantID, Filter: filter, CreatedAt: last.CreatedAt, ID: last.ID, ExpiresAt: now.Add(s.cursorTTL)})
		}
		return nil
	})
	return result, err
}

func (s *Service) attachTaskSources(ctx context.Context, tenantID string, views []TaskView) error {
	if s.issueSourceLookup == nil || len(views) == 0 {
		return nil
	}
	taskIDs := make([]string, 0, len(views))
	for i := range views {
		taskIDs = append(taskIDs, views[i].ID)
	}
	sources, err := s.issueSourceLookup.LookupByTaskIDs(ctx, tenantID, taskIDs)
	if err != nil {
		return err
	}
	for i := range views {
		if source, ok := sources[views[i].ID]; ok {
			views[i].Source = &source
		}
	}
	return nil
}

func taskView(record TaskRecord) (TaskView, error) {
	var constraints, requirements []string
	if err := json.Unmarshal(record.Constraints, &constraints); err != nil {
		return TaskView{}, err
	}
	if err := json.Unmarshal(record.Requirements, &requirements); err != nil {
		return TaskView{}, err
	}
	return TaskView{ID: record.ID, TenantID: record.TenantID, PublisherAgentVersionID: record.PublisherAgentVersionID, Type: record.Type, Title: record.Title, Problem: record.Problem, Constraints: constraints, Requirements: requirements, Deadline: record.Deadline, Status: record.Status, ClaimedBy: record.ClaimedBy, ActiveExecutionID: record.ActiveExecutionID, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, StateVersion: record.StateVersion}, nil
}

func filterDigest(query ListTasks) string {
	statuses := append([]domain.TaskStatus(nil), query.Statuses...)
	sort.Slice(statuses, func(i, j int) bool { return statuses[i] < statuses[j] })
	parts := make([]string, len(statuses))
	for i := range statuses {
		parts[i] = string(statuses[i])
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, ",") + "\x00" + query.Type + "\x00" + query.PublisherAgentVersionID))
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
