package main_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/publicsuffix"
)

func TestMain(m *testing.M) {
	exitCode := m.Run()

	if dbManager.conn != nil && dbManager.dbTemplateCreated {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		templateDBName := "hannibal_test_template"
		_, err := dbManager.conn.Exec(ctx, fmt.Sprintf("drop database if exists %s", templateDBName))
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to remove template database %s: %v", templateDBName, err)
		}
	}

	os.Exit(exitCode)
}

type dbManagerT struct {
	mutex sync.Mutex
	conn  *pgx.Conn

	dbCount           int64
	dbTemplateCreated bool
}

var dbManager dbManagerT

// ensurePool ensures a management connection exists. mutex must already be held.
func (dbm *dbManagerT) ensureConn(t *testing.T) {
	if dbm.conn != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var err error
	dbm.conn, err = pgx.Connect(ctx, "")
	require.NoError(t, err)
}

func (dbm *dbManagerT) createInitializedDB(t *testing.T) string {
	dbm.mutex.Lock()
	defer dbm.mutex.Unlock()
	dbm.ensureConn(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	templateDBName := "hannibal_test_template"
	if !dbm.dbTemplateCreated {
		_, err := dbm.conn.Exec(ctx, fmt.Sprintf("drop database if exists %s", templateDBName))
		require.NoError(t, err)

		_, err = dbm.conn.Exec(ctx, fmt.Sprintf("create database %s", templateDBName))
		require.NoError(t, err)

		execHannibal(t,
			"db", "init",
			"--database-dsn", fmt.Sprintf("database=%s", templateDBName),
		)

		dbm.dbTemplateCreated = true
	}

	dbm.dbCount += 1
	dbName := fmt.Sprintf("hannibal_test_%d", dbm.dbCount)

	_, err := dbm.conn.Exec(ctx, fmt.Sprintf("create database %s with template = %s", dbName, templateDBName))
	require.NoError(t, err)

	return dbName
}

func (dbm *dbManagerT) createEmptyDB(t *testing.T) string {
	dbm.mutex.Lock()
	defer dbm.mutex.Unlock()
	dbm.ensureConn(t)

	dbm.dbCount += 1
	dbName := fmt.Sprintf("hannibal_test_%d", dbm.dbCount)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := dbm.conn.Exec(ctx, fmt.Sprintf("create database %s", dbName))
	require.NoError(t, err)

	return dbName
}

func (dbm *dbManagerT) dropDB(t *testing.T, dbName string) {
	dbm.mutex.Lock()
	defer dbm.mutex.Unlock()
	dbm.ensureConn(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := dbm.conn.Exec(ctx, fmt.Sprintf("drop database %s", dbName))
	require.NoError(t, err)
}

func execHannibal(t *testing.T, args ...string) string {
	cmdPath := filepath.Join("tmp", "test", "hannibal")

	cmd := exec.Command(cmdPath, args...)
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "failed with output:\n%v", string(output))
	return string(output)
}

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
	testDB := dbManager.createInitializedDB(t)
	defer dbManager.dropDB(t, testDB)

	hs := startHannibal(t,
		"develop",
		map[string]string{
			"--project-path": filepath.Join("testdata", "testproject"),
			"--database-dsn": fmt.Sprintf("database=%s", testDB),
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
