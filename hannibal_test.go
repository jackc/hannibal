package main_test

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/publicsuffix"
)

type hannibalServer struct {
	process *os.Process
	addr    string
}

func startHannibal(t *testing.T, command string, args map[string]string) *hannibalServer {
	if _, ok := args["http-service-address"]; !ok {
		args["--http-service-address"] = "127.0.0.1:5000"
	}

	cmdPath := filepath.Join("tmp", "test", "hannibal")
	argv := []string{cmdPath, command}
	for k, v := range args {
		argv = append(argv, k, v)
	}
	procAttr := &os.ProcAttr{
		Files: []*os.File{nil, os.Stdout, os.Stderr},
	}

	process, err := os.StartProcess(cmdPath, argv, procAttr)
	require.NoError(t, err)

	serverActive := false
	for i := 0; i < 100; i++ {
		conn, err := net.DialTimeout("tcp", args["--http-service-address"], time.Second)
		if err == nil {
			err := conn.Close()
			require.NoError(t, err)
			serverActive = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.True(t, serverActive, "hannibal server appeared to start but is not listening")

	hs := &hannibalServer{
		process: process,
		addr:    args["--http-service-address"],
	}
	return hs
}

func stopHannibal(t *testing.T, hs *hannibalServer) {
	err := hs.process.Kill()
	require.NoError(t, err)

	waitDone := make(chan bool)
	waitErr := make(chan error)

	go func() {
		_, err := hs.process.Wait()
		if err != nil {
			waitErr <- err
		} else {
			waitDone <- true
		}
	}()

	select {
	case <-waitDone:
	case err := <-waitErr:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for hannibal to terminate")
	}
}

type browser struct {
	serverAddr string
	client     *http.Client
}

func newBrowser(t *testing.T, serverAddr string) *browser {
	options := cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	}
	jar, err := cookiejar.New(&options)
	require.NoError(t, err)

	client := &http.Client{Jar: jar}
	return &browser{
		serverAddr: serverAddr,
		client:     client,
	}
}

func (b *browser) get(t *testing.T, queryPath string) *http.Response {
	response, err := b.client.Get(fmt.Sprintf(`http://%s%s`, b.serverAddr, queryPath))
	require.NoError(t, err)
	return response
}

func readResponseBody(t *testing.T, r *http.Response) []byte {
	data, err := ioutil.ReadAll(r.Body)
	require.NoError(t, err)

	err = r.Body.Close()
	require.NoError(t, err)

	return data
}

func TestServePublicFiles(t *testing.T) {
	hs := startHannibal(t,
		"develop",
		map[string]string{
			"--project-path": filepath.Join("testdata", "testproject"),
			"--database-dsn": "database=hannibal_test_testapp",
		},
	)
	defer stopHannibal(t, hs)

	browser := newBrowser(t, hs.addr)

	response := browser.get(t, "/hello.html")
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	responseBody := readResponseBody(t, response)
	fileBody, err := ioutil.ReadFile(filepath.Join("testdata", "testproject", "public", "hello.html"))
	require.NoError(t, err)
	require.Equal(t, fileBody, responseBody)
}
