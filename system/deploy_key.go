package system

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/jackc/hannibal/db"
	"github.com/jackc/pgxutil"
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

func ValidateDeployment(ctx context.Context, userID int32, digest, signature []byte) (bool, error) {
	publicKeys, err := pgxutil.SelectAllByteSlice(ctx, db.Sys(ctx),
		fmt.Sprintf("select public_key from %s.deploy_keys where user_id = $1 and delete_time is null", db.GetConfig(ctx).SysSchema),
		userID,
	)
	if err != nil {
		return false, err
	}

	for _, pk := range publicKeys {
		if len(pk) != ed25519.PublicKeySize {
			continue // the database record of the public key must be corrupted somehow
		}
		if ed25519.Verify(ed25519.PublicKey(pk), digest, signature) {
			return true, nil
		}
	}

	return false, nil
}
