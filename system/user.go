package system

import (
	"context"
	"fmt"

	"github.com/jackc/hannibal/db"
)

func CreateUser(ctx context.Context, name string) (int32, error) {
	var id int32
	err := db.Sys(ctx).QueryRow(
		ctx,
		fmt.Sprintf("insert into %s.users (name) values ($1) returning id", db.GetConfig(ctx).SysSchema),
		name,
	).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}
