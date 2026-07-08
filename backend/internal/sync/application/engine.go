package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	syncdomain "agentguild.dev/agentguild/backend/internal/sync/domain"
)

const defaultSyncDeadline = 365 * 24 * time.Hour

type PublishSystemTaskInput struct {
	TenantID     string
	RequestID    string
	Type         string
	Title        string
	Problem      string
	Constraints  []string
	Requirements []string
	Deadline     time.Time
}

type ContentInput struct {
	Title        string
	Problem      string
	Constraints  []string
	Requirements []string
}

type TaskSink interface {
	PublishSystemTask(context.Context, PublishSystemTaskInput) (taskID string, err error)
	CancelSystemTask(ctx context.Context, tenantID, taskID, reason string) error
	UpdateSystemTaskContent(ctx context.Context, tenantID, taskID string, in ContentInput) (currentStatus string, err error)
	TaskStatus(ctx context.Context, tenantID, taskID string) (status string, err error)
}

type IssueSourceProvider interface {
	IssueSource(ctx context.Context, tenantID, sourceAuth string) (git.IssueSource, error)
}

type EngineOptions struct {
	DefaultDeadline time.Duration
	Now             func() time.Time
}

type SyncResult struct {
	Created   int
	Updated   int
	Skipped   int
	Cancelled int
	Failed    int
	Flagged   int
	Errors    []string
}

type Engine struct {
	rules   RuleRepository
	maps    MapRepository
	sink    TaskSink
	sources IssueSourceProvider
	opts    EngineOptions
}

func NewEngine(rules RuleRepository, maps MapRepository, sink TaskSink, sources IssueSourceProvider, opts EngineOptions) *Engine {
	if opts.DefaultDeadline <= 0 {
		opts.DefaultDeadline = defaultSyncDeadline
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Engine{rules: rules, maps: maps, sink: sink, sources: sources, opts: opts}
}

func (e *Engine) RunRule(ctx context.Context, tenantID, ruleID string) (SyncResult, error) {
	var result SyncResult
	rule, err := e.rules.Get(ctx, tenantID, ruleID)
	if err != nil {
		return result, err
	}
	source, err := e.sources.IssueSource(ctx, tenantID, rule.SourceAuth)
	if err != nil {
		return result, err
	}
	now := e.opts.Now()
	issues, err := source.ListIssues(ctx, rule.Repo, git.IssueFilter{
		State:  rule.IssueState,
		Labels: append([]string(nil), rule.IncludeLabels...),
	}, rule.LastSyncedAt)
	if err != nil {
		return result, err
	}
	for _, issue := range issues {
		if err := e.processIssue(ctx, rule, issue, now, &result); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, err.Error())
		}
	}
	if err := e.rules.TouchSynced(ctx, tenantID, ruleID, now); err != nil {
		return result, err
	}
	return result, nil
}

func (e *Engine) RunAllEnabled(ctx context.Context) (SyncResult, error) {
	var result SyncResult
	rules, err := e.rules.ListEnabledAllTenants(ctx)
	if err != nil {
		return result, err
	}
	for _, rule := range rules {
		next, err := e.RunRule(ctx, rule.TenantID, rule.ID)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s/%s: %v", rule.TenantID, rule.ID, err))
			continue
		}
		result.add(next)
	}
	return result, nil
}

func (e *Engine) processIssue(ctx context.Context, rule *syncdomain.Rule, issue git.Issue, now time.Time, result *SyncResult) error {
	if !rule.Matches(issue) {
		result.Skipped++
		return nil
	}
	mapping, err := e.maps.Get(ctx, rule.TenantID, rule.Repo, issue.Number)
	if err != nil && !errors.Is(err, syncdomain.ErrNotFound) {
		return err
	}
	if errors.Is(err, syncdomain.ErrNotFound) {
		return e.processUnmappedIssue(ctx, rule, issue, now, result)
	}
	return e.processMappedIssue(ctx, rule, issue, mapping, now, result)
}

func (e *Engine) processUnmappedIssue(ctx context.Context, rule *syncdomain.Rule, issue git.Issue, now time.Time, result *SyncResult) error {
	if issue.State == "closed" {
		result.Skipped++
		return nil
	}
	taskID, err := e.sink.PublishSystemTask(ctx, PublishSystemTaskInput{
		TenantID:  rule.TenantID,
		RequestID: requestID(rule, issue),
		Type:      rule.TaskType,
		Title:     issue.Title,
		Problem:   issue.Body,
		Deadline:  now.Add(e.opts.DefaultDeadline),
	})
	if err != nil {
		return err
	}
	result.Created++
	return e.maps.Upsert(ctx, mappingForIssue(rule, issue, taskID, now))
}

func (e *Engine) processMappedIssue(ctx context.Context, rule *syncdomain.Rule, issue git.Issue, mapping *Mapping, now time.Time, result *SyncResult) error {
	status, err := e.sink.TaskStatus(ctx, rule.TenantID, mapping.TaskID)
	if err != nil {
		return err
	}
	if issue.State == "closed" {
		if isOpenOrDraft(status) {
			if err := e.sink.CancelSystemTask(ctx, rule.TenantID, mapping.TaskID, "source issue closed"); err != nil {
				return err
			}
			result.Cancelled++
		} else {
			result.Flagged++
		}
	} else if rule.DedupeStrategy == syncdomain.DedupeUpdate && isOpenOrDraft(status) {
		if _, err := e.sink.UpdateSystemTaskContent(ctx, rule.TenantID, mapping.TaskID, ContentInput{
			Title:   issue.Title,
			Problem: issue.Body,
		}); err != nil {
			return err
		}
		result.Updated++
	} else {
		result.Skipped++
	}
	refreshMapping(mapping, issue, now)
	return e.maps.Upsert(ctx, mapping)
}

func (r *SyncResult) add(next SyncResult) {
	r.Created += next.Created
	r.Updated += next.Updated
	r.Skipped += next.Skipped
	r.Cancelled += next.Cancelled
	r.Failed += next.Failed
	r.Flagged += next.Flagged
	r.Errors = append(r.Errors, next.Errors...)
}

func mappingForIssue(rule *syncdomain.Rule, issue git.Issue, taskID string, now time.Time) *Mapping {
	return &Mapping{
		TenantID:     rule.TenantID,
		Repo:         rule.Repo,
		IssueNumber:  issue.Number,
		TaskID:       taskID,
		IssueState:   issue.State,
		IssueClosed:  issue.State == "closed",
		IssueURL:     issue.HTMLURL,
		LastSyncedAt: now,
	}
}

func refreshMapping(mapping *Mapping, issue git.Issue, now time.Time) {
	mapping.IssueState = issue.State
	mapping.IssueClosed = issue.State == "closed"
	mapping.IssueURL = issue.HTMLURL
	mapping.LastSyncedAt = now
}

func requestID(rule *syncdomain.Rule, issue git.Issue) string {
	return fmt.Sprintf("sync:%s:%s:%d", rule.ID, rule.Repo, issue.Number)
}

func isOpenOrDraft(status string) bool {
	return status == "open" || status == "draft"
}
