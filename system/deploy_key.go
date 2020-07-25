package system

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/jackc/hannibal/db"
)

func CreateDeployKey(ctx context.Context, userID int32) (int32, string, error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return 0, "", err
	}

	var id int32
	err = db.Sys(ctx).QueryRow(
		ctx,
		fmt.Sprintf("insert into %s.deploy_keys (user_id, public_key) values ($1, $2) returning id", db.GetConfig(ctx).SysSchema),
		userID, []byte(pubKey),
	).Scan(&id)
	if err != nil {
		return 0, "", err
	}

	return id, hex.EncodeToString([]byte(privKey)), nil
}
