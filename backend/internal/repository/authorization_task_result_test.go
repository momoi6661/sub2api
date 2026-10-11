package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAuthorizationTaskDetailResult(t *testing.T) {
	for _, returned := range []any{nil, []byte(`{"type":"auth_file","auth":{"email":"owner@example.com"},"custom":[1,2]}`)} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		repo := &authorizationTaskRepository{db: db}
		mock.ExpectExec("UPDATE authorization_tasks SET status='failed'").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT .* FROM authorization_tasks WHERE id=").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id", "account_id", "status", "stage", "summary", "events", "error_code", "error_message", "created_at", "started_at", "finished_at"}).AddRow(1, 2, "succeeded", "verifying", []byte(`{}`), []byte(`[]`), "", "", time.Now(), nil, nil))
		mock.ExpectQuery("SELECT result FROM authorization_tasks WHERE id=").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"result"}).AddRow(returned))
		task, err := repo.Get(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if returned == nil {
			if len(task.Result) != 0 {
				t.Fatal("old task should not have a return value")
			}
		} else if string(task.Result) != string(returned.([]byte)) || !json.Valid(task.Result) {
			t.Fatalf("return value changed: %s", task.Result)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}
