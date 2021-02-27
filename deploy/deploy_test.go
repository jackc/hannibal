package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageWriterWriteTo(t *testing.T) {
	pw, err := newPackageWriter("testdata", nil)
	require.NoError(t, err)

	buf := &bytes.Buffer{}
	size, err := pw.WriteTo(buf)
	require.NoError(t, err)
	assert.EqualValues(t, 435, size)
	assert.EqualValues(t, buf.Len(), size)

	digest := sha512.Sum512_256(buf.Bytes())
	assert.Equal(t, "2f976cb7f997d0c1cf09466d660e5304a17260e3d7b8016a3da0c3d02a924683", hex.EncodeToString(digest[:]))
}

func TestPackageWriterWriteToIgnoredPaths(t *testing.T) {
	pw, err := newPackageWriter("testdata", []string{"hello.txt"})
	require.NoError(t, err)

	buf := &bytes.Buffer{}
	size, err := pw.WriteTo(buf)
	require.NoError(t, err)
	assert.EqualValues(t, 389, size)
	assert.EqualValues(t, buf.Len(), size)

	digest := sha512.Sum512_256(buf.Bytes())
	assert.Equal(t, "6b5ec6a7dce5c40f35d138a7fdc416e3a9cd58ff322003128ffb35ec61323bfc", hex.EncodeToString(digest[:]))
}

func makePackage(t *testing.T, path string, ignorePaths []string) (io.ReadSeeker, []byte) {
	pw, err := newPackageWriter("testdata", ignorePaths)
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

func assertEqualFiles(t testing.TB, expectedPath, actualPath string) bool {
	t.Helper()

	expectedStat, err := os.Stat(expectedPath)
	if !assert.NoError(t, err) {
		return false
	}

	actualStat, err := os.Stat(actualPath)
	if !assert.NoError(t, err) {
		return false
	}

	if !assert.Equalf(t, expectedStat.Mode(), actualStat.Mode(), "files have different modes") {
		return false
	}

	expectedBytes, err := ioutil.ReadFile(expectedPath)
	if !assert.NoError(t, err) {
		return false
	}

	actualBytes, err := ioutil.ReadFile(actualPath)
	if !assert.NoError(t, err) {
		return false
	}

	if !assert.Equalf(t, expectedBytes, actualBytes, "file contents differ") {
		return false
	}

	return true
}

func TestUnpack(t *testing.T) {
	pkg, digest := makePackage(t, "testdata", nil)

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	signature := ed25519.Sign(privateKey, digest)

	tempDir := t.TempDir()
	err = Unpack(pkg, signature, tempDir, []ed25519.PublicKey{publicKey})
	require.NoError(t, err)

	for _, f := range []string{
		"hello.txt",
		"sql/first.sql",
		"sql/manifest.conf",
	} {
		assertEqualFiles(t, filepath.Join("testdata", f), filepath.Join(tempDir, f))
	}
}

func TestUnpackRejectsInvalidSignature(t *testing.T) {
	pkg, digest := makePackage(t, "testdata", nil)

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	privateKey[0] = privateKey[0] ^ 1 // Flip one bit
	signature := ed25519.Sign(privateKey, digest)

	tempDir := t.TempDir()
	err = Unpack(pkg, signature, tempDir, []ed25519.PublicKey{publicKey})
	require.EqualError(t, err, ErrInvalidSignature.Error())
}

func TestUnpackToMissingDirectory(t *testing.T) {
	pkg, digest := makePackage(t, "testdata", nil)

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	signature := ed25519.Sign(privateKey, digest)

	err = Unpack(pkg, signature, "/some/missing/directory", []ed25519.PublicKey{publicKey})
	require.Error(t, err)
}

func makePackageFunc(t *testing.T, f func(tw *tar.Writer)) (io.ReadSeeker, []byte) {
	buf := &bytes.Buffer{}
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	f(tw)

	err := tw.Close()
	require.NoError(t, err)

	err = gw.Close()
	require.NoError(t, err)

	pkg := bytes.NewReader(buf.Bytes())

	hashDigest := sha512.New()
	_, err = pkg.WriteTo(hashDigest)
	require.NoError(t, err)
	_, _ = pkg.Seek(0, io.SeekStart)

	return pkg, hashDigest.Sum(nil)
}

func TestUnpackRejectsSymlinks(t *testing.T) {
	pkg, digest := makePackageFunc(t, func(tw *tar.Writer) {
		err := tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeSymlink,
			Name:     "sym",
			Size:     0,
			Mode:     0644,
		})
		require.NoError(t, err)
	})

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	signature := ed25519.Sign(privateKey, digest)

	tempDir := t.TempDir()
	err = Unpack(pkg, signature, tempDir, []ed25519.PublicKey{publicKey})
	require.EqualError(t, err, ErrInvalidPackage.Error())
}

func TestUnpackRejectsDirectoryTraversal(t *testing.T) {
	pkg, digest := makePackageFunc(t, func(tw *tar.Writer) {
		err := tw.WriteHeader(&tar.Header{
			Name: "../../../etc/passwd-ish", // Don't use actual path to /etc/passwd just in case someone runs this as root and the test fails.
			Size: 30,
			Mode: 0600,
		})
		require.NoError(t, err)

		tw.Write([]byte("All your base are belong to us"))
	})

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	signature := ed25519.Sign(privateKey, digest)

	tempDir := t.TempDir()
	err = Unpack(pkg, signature, tempDir, []ed25519.PublicKey{publicKey})
	require.EqualError(t, err, ErrInvalidPackage.Error())
}
