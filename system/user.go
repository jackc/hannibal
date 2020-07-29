package system

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/jackc/hannibal/db"
	"github.com/jackc/pgx/v4"
)

func CreateUser(ctx context.Context, username string) (int32, error) {
	var id int32
	err := db.Sys(ctx).QueryRow(
		ctx,
		fmt.Sprintf(
			"insert into %s.users (username, creation_time, last_update_time) values ($1, now(), now()) returning id",
			db.QuoteSchema(db.GetConfig(ctx).SysSchema),
		),
		username,
	).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func AuthenticateUserByAPIKeyString(ctx context.Context, hexAPIKey string) (int32, error) {
	apiKey, err := hex.DecodeString(hexAPIKey)
	if err != nil {
		return 0, pgx.ErrNoRows // This should be treated as not found.
	}

	var id int32
	err = db.Sys(ctx).QueryRow(
		ctx,
		fmt.Sprintf(
			"select u.id from %[1]s.users u join %[1]s.api_keys k on u.id = k.user_id where k.digest = $1 and u.delete_time is null and k.delete_time is null",
			db.QuoteSchema(db.GetConfig(ctx).SysSchema),
		),
		digestAPIKey(apiKey),
	).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}
