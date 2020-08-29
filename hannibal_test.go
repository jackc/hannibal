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
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
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
	cmdPath := filepath.Join("tmp", "test", "bin", "hannibal")

	cmd := exec.Command(cmdPath, args...)
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "failed with output:\n%v", string(output))
	return string(output)
}

type hannibalInstance struct {
	dbName      string
	databaseDSN string

	httpProcess *hannibalProcess
	httpAddr    string

	appPath     string
	projectPath string
}

func (hi *hannibalInstance) systemCreateUser(t *testing.T, username string) {
	execHannibal(t,
		"system", "create-user",
		"--database-dsn", hi.databaseDSN,
		"-u", username,
	)
}

func (hi *hannibalInstance) systemCreateAPIKey(t *testing.T, username string) string {
	output := execHannibal(t,
		"system", "create-api-key",
		"--database-dsn", hi.databaseDSN,
		"-u", username,
	)

	match := regexp.MustCompile(`[0-9a-f]+`).FindString(output)
	require.NotEmpty(t, match)
	return match
}

func (hi *hannibalInstance) systemCreateDeployKey(t *testing.T, username string) string {
	output := execHannibal(t,
		"system", "create-deploy-key",
		"--database-dsn", hi.databaseDSN,
		"-u", username,
	)

	match := regexp.MustCompile(`[0-9a-f]+`).FindString(output)
	require.NotEmpty(t, match)
	return match
}

func (hi *hannibalInstance) develop(t *testing.T) {
	if hi.httpProcess != nil {
		t.Fatal("process already started")
	}

	if hi.httpAddr == "" {
		hi.httpAddr = "127.0.0.1:5000"
	}

	hi.httpProcess = spawnHannibal(t,
		"develop",
		"--http-service-address", hi.httpAddr,
		"--database-dsn", hi.databaseDSN,
		"--project-path", hi.projectPath,
	)

	waitForListeningTCPServer(t, hi.httpAddr)
}

func (hi *hannibalInstance) serve(t *testing.T) {
	if hi.httpProcess != nil {
		t.Fatal("process already started")
	}

	if hi.httpAddr == "" {
		hi.httpAddr = "127.0.0.1:5000"
	}

	hi.httpProcess = spawnHannibal(t,
		"serve",
		"--http-service-address", hi.httpAddr,
		"--database-dsn", hi.databaseDSN,
		"--app-path", hi.appPath,
	)

	waitForListeningTCPServer(t, hi.httpAddr)
}

func waitForListeningTCPServer(t *testing.T, addr string) {
	serverActive := false
	for i := 0; i < 100; i++ {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			err := conn.Close()
			require.NoError(t, err)
			serverActive = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.True(t, serverActive, "hannibal server appeared to start but is not listening")
}

func (hi *hannibalInstance) deploy(t *testing.T, apiKey, deployKey string) {
	if hi.httpProcess == nil || hi.httpAddr == "" {
		t.Fatal("no http process to deploy to")
	}

	execHannibal(t, "deploy",
		fmt.Sprintf(`http://%s/`, hi.httpAddr),
		"--project-path", hi.projectPath,
		"--api-key", apiKey,
		"--deploy-key", deployKey,
	)
}

func (hi *hannibalInstance) stop(t *testing.T) {
	if hi.httpProcess == nil {
		t.Fatal("no process to stop")
	}

	hi.httpProcess.stop(t)
	hi.httpProcess = nil
}

type hannibalProcess struct {
	process *os.Process
}

func (hp *hannibalProcess) stop(t *testing.T) {
	err := hp.process.Kill()
	require.NoError(t, err)

	waitDone := make(chan bool)
	waitErr := make(chan error)

	go func() {
		_, err := hp.process.Wait()
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

func spawnHannibal(t *testing.T, args ...string) *hannibalProcess {
	cmdPath := filepath.Join("tmp", "test", "bin", "hannibal")
	argv := []string{cmdPath}
	argv = append(argv, args...)
	procAttr := &os.ProcAttr{
		Files: []*os.File{nil, os.Stdout, os.Stderr},
	}

	process, err := os.StartProcess(cmdPath, argv, procAttr)
	require.NoError(t, err)

	hp := &hannibalProcess{
		process: process,
	}

	return hp
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

func TestDevelopAutoLoads(t *testing.T) {
	testDB := dbManager.createInitializedDB(t)
	defer dbManager.dropDB(t, testDB)

	appDir := t.TempDir()
	projectPath := filepath.Join(appDir, "project")
	err := copy.Copy(filepath.Join("testdata", "testproject"), projectPath)
	require.NoError(t, err)

	hi := &hannibalInstance{
		dbName:      testDB,
		databaseDSN: fmt.Sprintf("database=%s", testDB),
		projectPath: projectPath,
	}

	hi.develop(t)
	defer hi.stop(t)

	browser := newBrowser(t, hi.httpAddr)

	response := browser.get(t, `/hello?name=Jack`)
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	responseBody := string(readResponseBody(t, response))
	assert.Contains(t, responseBody, "Hello, Jack!")
}

func TestDevelopAutoReloadsOnSQLChanges(t *testing.T) {
	testDB := dbManager.createInitializedDB(t)
	defer dbManager.dropDB(t, testDB)

	appDir := t.TempDir()
	projectPath := filepath.Join(appDir, "project")
	err := copy.Copy(filepath.Join("testdata", "testproject"), projectPath)
	require.NoError(t, err)

	hi := &hannibalInstance{
		dbName:      testDB,
		databaseDSN: fmt.Sprintf("database=%s", testDB),
		projectPath: projectPath,
	}

	hi.develop(t)
	defer hi.stop(t)

	newSQL := `create function hello(
		out template text,
		out template_data jsonb
	)
	language plpgsql as $$
	begin
		select
			'hello.html',
			jsonb_build_object(
				'name', 'Alice'
			)
		into template, template_data;
	end;
	$$;
	`

	err = ioutil.WriteFile(filepath.Join(projectPath, "sql", "hello.sql"), []byte(newSQL), 0644)
	require.NoError(t, err)

	browser := newBrowser(t, hi.httpAddr)

	var responseBody string
	// Allow a little time for the changed file to be detected.
	for i := 0; i < 20; i++ {
		response := browser.get(t, `/hello?name=Jack`)
		require.EqualValues(t, http.StatusOK, response.StatusCode)
		responseBody = string(readResponseBody(t, response))
		if strings.Contains(responseBody, "Hello, Alice!") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.Contains(t, responseBody, "Hello, Alice!")
}

func TestDevelopAutoReloadsOnTemplateChanges(t *testing.T) {

	testDB := dbManager.createInitializedDB(t)
	defer dbManager.dropDB(t, testDB)

	appDir := t.TempDir()
	projectPath := filepath.Join(appDir, "project")
	err := copy.Copy(filepath.Join("testdata", "testproject"), projectPath)
	require.NoError(t, err)

	hi := &hannibalInstance{
		dbName:      testDB,
		databaseDSN: fmt.Sprintf("database=%s", testDB),
		projectPath: projectPath,
	}

	hi.develop(t)
	defer hi.stop(t)

	newTemplate := `<html>
	<body>
		<p>
			Hello, Alice!
		</p>
	</body>
	</html>
	`

	err = ioutil.WriteFile(filepath.Join(projectPath, "template", "hello.html"), []byte(newTemplate), 0644)
	require.NoError(t, err)

	browser := newBrowser(t, hi.httpAddr)

	var responseBody string
	// Allow a little time for the changed file to be detected.
	for i := 0; i < 20; i++ {
		response := browser.get(t, `/hello?name=Jack`)
		require.EqualValues(t, http.StatusOK, response.StatusCode)
		responseBody = string(readResponseBody(t, response))
		if strings.Contains(responseBody, "Hello, Alice!") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.Contains(t, responseBody, "Hello, Alice!")
}

func TestDevelopPublicFiles(t *testing.T) {
	testDB := dbManager.createInitializedDB(t)
	defer dbManager.dropDB(t, testDB)

	hi := &hannibalInstance{
		dbName:      testDB,
		databaseDSN: fmt.Sprintf("database=%s", testDB),
		projectPath: filepath.Join("testdata", "testproject"),
	}

	hi.develop(t)
	defer hi.stop(t)

	browser := newBrowser(t, hi.httpAddr)

	response := browser.get(t, "/hello.html")
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	responseBody := readResponseBody(t, response)
	fileBody, err := ioutil.ReadFile(filepath.Join("testdata", "testproject", "public", "hello.html"))
	require.NoError(t, err)
	require.Equal(t, fileBody, responseBody)
}

func TestServePublicFiles(t *testing.T) {
	testDB := dbManager.createInitializedDB(t)
	defer dbManager.dropDB(t, testDB)

	appDir := t.TempDir()

	hi := &hannibalInstance{
		dbName:      testDB,
		databaseDSN: fmt.Sprintf("database=%s", testDB),
		appPath:     appDir,
		projectPath: filepath.Join("testdata", "testproject"),
	}

	hi.serve(t)
	defer hi.stop(t)

	hi.systemCreateUser(t, "test")
	apiKey := hi.systemCreateAPIKey(t, "test")
	deployKey := hi.systemCreateDeployKey(t, "test")
	hi.deploy(t, apiKey, deployKey)

	browser := newBrowser(t, hi.httpAddr)

	response := browser.get(t, "/hello.html")
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	responseBody := readResponseBody(t, response)
	fileBody, err := ioutil.ReadFile(filepath.Join("testdata", "testproject", "public", "hello.html"))
	require.NoError(t, err)
	require.Equal(t, fileBody, responseBody)
}

func TestQueryArgs(t *testing.T) {
	testDB := dbManager.createInitializedDB(t)
	defer dbManager.dropDB(t, testDB)

	appDir := t.TempDir()

	hi := &hannibalInstance{
		dbName:      testDB,
		databaseDSN: fmt.Sprintf("database=%s", testDB),
		appPath:     appDir,
		projectPath: filepath.Join("testdata", "testproject"),
	}

	hi.serve(t)
	defer hi.stop(t)

	hi.systemCreateUser(t, "test")
	apiKey := hi.systemCreateAPIKey(t, "test")
	deployKey := hi.systemCreateDeployKey(t, "test")
	hi.deploy(t, apiKey, deployKey)

	browser := newBrowser(t, hi.httpAddr)

	response := browser.get(t, "/hello?name=Jack")
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	responseBody := string(readResponseBody(t, response))
	assert.Contains(t, responseBody, "Hello, Jack")
}

func TestCookieSession(t *testing.T) {
	testDB := dbManager.createInitializedDB(t)
	defer dbManager.dropDB(t, testDB)

	appDir := t.TempDir()

	hi := &hannibalInstance{
		dbName:      testDB,
		databaseDSN: fmt.Sprintf("database=%s", testDB),
		appPath:     appDir,
		projectPath: filepath.Join("testdata", "testproject"),
	}

	hi.serve(t)
	defer hi.stop(t)

	hi.systemCreateUser(t, "test")
	apiKey := hi.systemCreateAPIKey(t, "test")
	deployKey := hi.systemCreateDeployKey(t, "test")
	hi.deploy(t, apiKey, deployKey)

	browser := newBrowser(t, hi.httpAddr)

	for i := 1; i < 5; i++ {
		response := browser.get(t, "/hello")
		require.EqualValues(t, http.StatusOK, response.StatusCode)
		responseBody := string(readResponseBody(t, response))
		assert.Contains(t, responseBody, fmt.Sprintf("%d times", i))
	}
}
