package deploy

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageWriterWriteTo(t *testing.T) {
	pw, err := newPackageWriter("testdata")
	require.NoError(t, err)

	buf := &bytes.Buffer{}
	size, err := pw.WriteTo(buf)
	require.NoError(t, err)
	assert.EqualValues(t, 435, size)
	assert.EqualValues(t, buf.Len(), size)

	digest := sha512.Sum512_256(buf.Bytes())
	assert.Equal(t, "2f976cb7f997d0c1cf09466d660e5304a17260e3d7b8016a3da0c3d02a924683", hex.EncodeToString(digest[:]))
}
