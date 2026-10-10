package service

import (
	"context"
	"time"
)

// Task history deliberately contains no mailbox secrets, scripts or OAuth credentials.
type AuthorizationTask struct {
	ID           int64                    `json:"id"`
	AccountID    int64                    `json:"account_id"`
	AccountName  string                   `json:"account_name"`
	LoginEmail   string                   `json:"login_email"`
	ProfileID    string                   `json:"profile_id"`
	ProfileName  string                   `json:"profile_name"`
	Source       string                   `json:"source"`
	Status       string                   `json:"status"`
	Stage        string                   `json:"stage"`
	Events       []AuthorizationTaskEvent `json:"events"`
	ErrorCode    string                   `json:"error_code"`
	ErrorMessage string                   `json:"error_message"`
	CreatedAt    time.Time                `json:"created_at"`
	StartedAt    *time.Time               `json:"started_at"`
	FinishedAt   *time.Time               `json:"finished_at"`
}

type AuthorizationTaskEvent struct {
	Stage string    `json:"stage"`
	At    time.Time `json:"at"`
}

type AuthorizationTaskPage struct {
	Items       []AuthorizationTask `json:"items"`
	Total       int64               `json:"total"`
	ActiveCount int64               `json:"active_count"`
}

type AuthorizationTaskRepository interface {
	Create(context.Context, []*AuthorizationTask) error
	Claim(context.Context) (*AuthorizationTask, error)
	Report(context.Context, int64, string) error
	Finish(context.Context, int64, string, string, string) error
	List(context.Context, string, string, int, int) (*AuthorizationTaskPage, error)
	Get(context.Context, int64) (*AuthorizationTask, error)
}
