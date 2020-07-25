package system

import (
	"context"
	"fmt"

	"github.com/jackc/hannibal/db"
)

func CreateUser(ctx context.Context, username string) (int32, error) {
	var id int32
	err := db.Sys(ctx).QueryRow(
		ctx,
		fmt.Sprintf("insert into %s.users (username, creation_time, last_update_time) values ($1, now(), now()) returning id", db.GetConfig(ctx).SysSchema),
		username,
	).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}
