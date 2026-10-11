package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type authorizationTaskRepository struct{ db *sql.DB }

func NewAuthorizationTaskRepository(db *sql.DB) service.AuthorizationTaskRepository {
	return &authorizationTaskRepository{db}
}

const authTaskColumns = `id, account_id, status, stage, summary, events, error_code, error_message, created_at, started_at, finished_at`

func scanAuthTask(row interface{ Scan(...any) error }) (*service.AuthorizationTask, error) {
	var t service.AuthorizationTask
	var summary, events []byte
	err := row.Scan(&t.ID, &t.AccountID, &t.Status, &t.Stage, &summary, &events, &t.ErrorCode, &t.ErrorMessage, &t.CreatedAt, &t.StartedAt, &t.FinishedAt)
	if err != nil {
		return nil, err
	}
	var snapshot struct {
		AccountName string `json:"account_name"`
		LoginEmail  string `json:"login_email"`
		ProfileID   string `json:"profile_id"`
		ProfileName string `json:"profile_name"`
		Source      string `json:"source"`
	}
	if err = json.Unmarshal(summary, &snapshot); err != nil {
		return nil, err
	}
	t.AccountName, t.LoginEmail, t.ProfileID, t.ProfileName, t.Source = snapshot.AccountName, snapshot.LoginEmail, snapshot.ProfileID, snapshot.ProfileName, snapshot.Source
	if err = json.Unmarshal(events, &t.Events); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *authorizationTaskRepository) Create(ctx context.Context, tasks []*service.AuthorizationTask) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Expired execution never remains permanently active after a process interruption.
	if _, err = tx.ExecContext(ctx, expireAuthTasks); err != nil {
		return err
	}
	for _, t := range tasks {
		t.CreatedAt = time.Now().UTC()
		t.Stage = "preparing"
		snapshot, err := json.Marshal(map[string]string{"account_name": t.AccountName, "login_email": t.LoginEmail, "profile_id": t.ProfileID, "profile_name": t.ProfileName, "source": t.Source})
		if err != nil {
			return err
		}
		if t.Status == "running" {
			now := t.CreatedAt
			t.StartedAt = &now
		}
		t.Events = []service.AuthorizationTaskEvent{{Stage: "preparing", At: t.CreatedAt}}
		events, _ := json.Marshal(t.Events)
		err = tx.QueryRowContext(ctx, `INSERT INTO authorization_tasks (account_id,status,summary,events,created_at,started_at) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, t.AccountID, t.Status, string(snapshot), string(events), t.CreatedAt, t.StartedAt).Scan(&t.ID)
		if err != nil {
			var pg *pq.Error
			if errors.As(err, &pg) && pg.Code == "23505" {
				return &service.ReauthExecutionError{Code: "TASK_ALREADY_RUNNING", Message: "An account already has a queued or running authorization task"}
			}
			return err
		}
	}
	return tx.Commit()
}

const expireAuthTasks = `UPDATE authorization_tasks SET status='failed', error_code='TASK_INTERRUPTED', error_message='Task timed out or the server stopped. Retry manually.', finished_at=started_at + INTERVAL '660 seconds' WHERE status='running' AND started_at < NOW() - INTERVAL '660 seconds'`

func (r *authorizationTaskRepository) Claim(ctx context.Context) (*service.AuthorizationTask, error) {
	if _, err := r.db.ExecContext(ctx, expireAuthTasks); err != nil {
		return nil, err
	}
	t, err := scanAuthTask(r.db.QueryRowContext(ctx, `UPDATE authorization_tasks SET status='running',started_at=NOW() WHERE id=(SELECT id FROM authorization_tasks WHERE status='queued' ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING `+authTaskColumns))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

func (r *authorizationTaskRepository) Report(ctx context.Context, id int64, stage string) error {
	event, _ := json.Marshal([]service.AuthorizationTaskEvent{{Stage: stage, At: time.Now().UTC()}})
	_, err := r.db.ExecContext(ctx, `UPDATE authorization_tasks SET stage=$2,events=events || $3::jsonb WHERE id=$1 AND status='running' AND stage<>$2`, id, stage, string(event))
	return err
}
func (r *authorizationTaskRepository) Finish(ctx context.Context, id int64, status, code, message string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE authorization_tasks SET status=$2,error_code=$3,error_message=$4,finished_at=NOW() WHERE id=$1 AND status='running'`, id, status, code, message)
	return err
}
func (r *authorizationTaskRepository) Cancel(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE authorization_tasks SET status='cancelled',error_code='TASK_CANCELLED',error_message='Authorization task cancelled by operator.',finished_at=NOW() WHERE id=$1 AND status IN ('queued','running')`, id)
	return err
}
func (r *authorizationTaskRepository) SaveResult(ctx context.Context, id int64, result json.RawMessage) error {
	_, err := r.db.ExecContext(ctx, `UPDATE authorization_tasks SET result=$2::jsonb WHERE id=$1 AND status='running'`, id, string(result))
	return err
}
func (r *authorizationTaskRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM authorization_tasks WHERE id=$1`, id)
	return err
}
func (r *authorizationTaskRepository) Get(ctx context.Context, id int64) (*service.AuthorizationTask, error) {
	if _, err := r.db.ExecContext(ctx, expireAuthTasks); err != nil {
		return nil, err
	}
	task, err := scanAuthTask(r.db.QueryRowContext(ctx, `SELECT `+authTaskColumns+` FROM authorization_tasks WHERE id=$1`, id))
	if err != nil {
		return nil, err
	}
	var result []byte
	err = r.db.QueryRowContext(ctx, `SELECT result FROM authorization_tasks WHERE id=$1`, id).Scan(&result)
	task.Result = result
	return task, err
}
func (r *authorizationTaskRepository) List(ctx context.Context, q, status string, page, size int) (*service.AuthorizationTaskPage, error) {
	if _, err := r.db.ExecContext(ctx, expireAuthTasks); err != nil {
		return nil, err
	}
	q = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
	filter := ` WHERE ($1='' OR status=$1 OR ($1='active' AND status IN ('queued','running'))) AND ($2='' OR summary->>'account_name' ILIKE $3 OR summary->>'login_email' ILIKE $3 OR summary->>'profile_name' ILIKE $3 OR id::text=$2 OR error_code ILIKE $3)`
	result := &service.AuthorizationTaskPage{Items: []service.AuthorizationTask{}}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM authorization_tasks`+filter, status, q, "%"+q+"%").Scan(&result.Total); err != nil {
		return nil, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM authorization_tasks WHERE status IN ('queued','running')`).Scan(&result.ActiveCount); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+authTaskColumns+` FROM authorization_tasks`+filter+` ORDER BY id DESC LIMIT $4 OFFSET $5`, status, q, "%"+q+"%", size, (page-1)*size)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		t, err := scanAuthTask(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, *t)
	}
	return result, rows.Err()
}
