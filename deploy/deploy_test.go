package deploy

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"io"
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

func makePackage(t *testing.T, path string) (io.ReadSeeker, []byte) {
	pw, err := newPackageWriter("testdata")
	require.NoError(t, err)

	buf := &bytes.Buffer{}
	_, err = pw.WriteTo(buf)
	require.NoError(t, err)

	pkg := bytes.NewReader(buf.Bytes())

	hashDigest := sha512.New()
	_, err = pkg.WriteTo(hashDigest)
	require.NoError(t, err)
	_, _ = pkg.Seek(0, io.SeekStart)

	return pkg, hashDigest.Sum(nil)
}

func TestUnpack(t *testing.T) {
	pkg, digest := makePackage(t, "testdata")

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	signature := ed25519.Sign(privateKey, digest)

	tempDir := t.TempDir()
	err = Unpack(pkg, signature, tempDir, []ed25519.PublicKey{publicKey})
	require.NoError(t, err)
}

func TestUnpackRejectsInvalidSignature(t *testing.T) {
	pkg, digest := makePackage(t, "testdata")

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	privateKey[0] = privateKey[0] ^ 1 // Flip one bit
	signature := ed25519.Sign(privateKey, digest)

	tempDir := t.TempDir()
	err = Unpack(pkg, signature, tempDir, []ed25519.PublicKey{publicKey})
	require.EqualError(t, err, ErrInvalidSignature.Error())
}
