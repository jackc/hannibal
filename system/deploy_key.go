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
		fmt.Sprintf("insert into %s.deploy_keys (user_id, public_key, creation_time) values ($1, $2, now()) returning id", db.GetConfig(ctx).SysSchema),
		userID, []byte(pubKey),
	).Scan(&id)
	if err != nil {
		return 0, "", err
	}

	return id, hex.EncodeToString(privKey.Seed()), nil
}

func GetDeployPublicKeysForUserID(ctx context.Context, userID int32) ([]ed25519.PublicKey, error) {
	var publicKeys []ed25519.PublicKey

	rows, err := db.Sys(ctx).Query(
		ctx,
		fmt.Sprintf("select public_key from %s.deploy_keys where user_id = $1 and delete_time is null", db.GetConfig(ctx).SysSchema),
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var buf []byte
		err := rows.Scan(&buf)
		if err != nil {
			return nil, err
		}

		// Ensure database record of the public key is the correct size. Otherwise a later attempt to use the key would panic.
		if len(buf) == ed25519.PublicKeySize {
			publicKeys = append(publicKeys, ed25519.PublicKey(buf))
		}
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return publicKeys, nil
}
