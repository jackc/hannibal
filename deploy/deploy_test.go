package deploy_test

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"testing"

	"github.com/jackc/hannibal/deploy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPackage(t *testing.T) {
	buf := &bytes.Buffer{}
	size, digest, err := deploy.BuildPackage(buf, "testdata")
	require.NoError(t, err)

	assert.EqualValues(t, 109, size)
	assert.EqualValues(t, buf.Len(), size)

	assert.Equal(t, "3180ca660ceddbd97e2c4c57afb47c6509ded0ae809e1b0d1380d648f6d472dc", hex.EncodeToString(digest))
	writtenDigest := sha512.Sum512_256(buf.Bytes())
	assert.EqualValues(t, digest, writtenDigest[:])
}
