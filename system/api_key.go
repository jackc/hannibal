package system

import (
	"context"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/jackc/hannibal/db"
)

func CreateAPIKey(ctx context.Context, userID int32) (int32, string, error) {
	apiKey := make([]byte, 24)
	_, err := io.ReadFull(rand.Reader, apiKey)
	if err != nil {
		return 0, "", err
	}

	digest := digestAPIKey(apiKey)

	var id int32
	err = db.Sys(ctx).QueryRow(
		ctx,
		fmt.Sprintf("insert into %s.api_keys (user_id, digest, creation_time) values ($1, $2, now()) returning id", db.GetConfig(ctx).SysSchema),
		userID, digest,
	).Scan(&id)
	if err != nil {
		return 0, "", err
	}

	return id, hex.EncodeToString(apiKey), nil
}

func digestAPIKey(apiKey []byte) []byte {
	digest := sha512.Sum512_256(apiKey)
	return digest[:]
}
