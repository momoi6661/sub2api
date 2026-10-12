package repository

import (
	"context"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSetErrorNotifiesAutomaticAuthOnlyAfterPersistence(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved", true: "write_failed"}[fail], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })
			events := service.NewAccountErrorEvents()
			called := false
			events.SetHandler(func(id int64) { require.Equal(t, int64(7), id); called = true })
			repo := ProvideAccountRepository(client, db, nil, events)
			update := mock.ExpectExec(`UPDATE "accounts" SET`)
			if fail {
				update.WillReturnError(errors.New("database write failed"))
			} else {
				update.WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`INSERT INTO scheduler_outbox`).WillReturnResult(sqlmock.NewResult(1, 1))
			}
			err = repo.SetError(context.Background(), 7, "401 unauthorized")
			if fail {
				require.Error(t, err)
				require.False(t, called)
			} else {
				require.NoError(t, err)
				require.True(t, called)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
