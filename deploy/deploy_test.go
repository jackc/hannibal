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

func TestPackageWriteTo(t *testing.T) {
	pkg, err := deploy.NewPackage("testdata")
	require.NoError(t, err)

	buf := &bytes.Buffer{}
	size, err := pkg.WriteTo(buf)
	require.NoError(t, err)
	assert.EqualValues(t, 435, size)
	assert.EqualValues(t, buf.Len(), size)

	digest := sha512.Sum512_256(buf.Bytes())
	assert.Equal(t, "2f976cb7f997d0c1cf09466d660e5304a17260e3d7b8016a3da0c3d02a924683", hex.EncodeToString(digest[:]))
}
