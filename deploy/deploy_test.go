package deploy_test

import (
	"bytes"
	"crypto/sha256"
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

	expectedDigest, err := hex.DecodeString("3a66192fa03a019806fddc4d537aad9380730dd258768ca3b3ae78ba3476c192")
	require.NoError(t, err)
	assert.EqualValues(t, expectedDigest, digest)
	writtenDigest := sha256.Sum256(buf.Bytes())
	assert.EqualValues(t, digest, writtenDigest[:])
}
