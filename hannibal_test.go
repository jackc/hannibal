package main_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
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
		_, err := dbManager.conn.Exec(ctx, fmt.Sprintf("drop database if exists %s with (force)", templateDBName))
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

func (dbm *dbManagerT) createInitializedDB(t *testing.T, projectPath string) string {
	dbm.mutex.Lock()
	defer dbm.mutex.Unlock()
	dbm.ensureConn(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	templateDBName := "hannibal_test_template"
	if !dbm.dbTemplateCreated {
		_, err := dbm.conn.Exec(ctx, fmt.Sprintf("drop database if exists %s with (force)", templateDBName))
		require.NoError(t, err)

		_, err = dbm.conn.Exec(ctx, fmt.Sprintf("create database %s", templateDBName))
		require.NoError(t, err)

		execHannibal(t,
			"db", "init",
			"--database-dsn", fmt.Sprintf("database=%s", templateDBName),
		)

		execHannibal(t,
			"db", "migrate",
			"--database-dsn", fmt.Sprintf("database=%s", templateDBName),
			"--project-path", projectPath,
		)

		dbm.dbTemplateCreated = true
	}

	dbm.dbCount += 1
	dbName := fmt.Sprintf("hannibal_test_%d", dbm.dbCount)

	_, err := dbm.conn.Exec(ctx, fmt.Sprintf("drop database if exists %s with (force)", dbName))
	require.NoError(t, err)
	_, err = dbm.conn.Exec(ctx, fmt.Sprintf("create database %s with template = %s", dbName, templateDBName))
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

	_, err := dbm.conn.Exec(ctx, fmt.Sprintf("drop database if exists %s with (force)", dbName))
	require.NoError(t, err)

	_, err = dbm.conn.Exec(ctx, fmt.Sprintf("create database %s", dbName))
	require.NoError(t, err)

	return dbName
}

func (dbm *dbManagerT) dropDB(t *testing.T, dbName string) {
	dbm.mutex.Lock()
	defer dbm.mutex.Unlock()
	dbm.ensureConn(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := dbm.conn.Exec(ctx, fmt.Sprintf("drop database %s with (force)", dbName))
	require.NoError(t, err)
}

func execHannibal(t *testing.T, args ...string) string {
	cmdPath := filepath.Join("tmp", "test", "bin", "hannibal")

	cmd := exec.Command(cmdPath, args...)
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "failed with output:\n%v", string(output))
	return string(output)
}

var httpPort int64 = 5000

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
		port := atomic.AddInt64(&httpPort, 1)
		hi.httpAddr = fmt.Sprintf("127.0.0.1:%d", port)
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
		port := atomic.AddInt64(&httpPort, 1)
		hi.httpAddr = fmt.Sprintf("127.0.0.1:%d", port)
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

func (hi *hannibalInstance) migrate(t *testing.T) {
	execHannibal(t, "db", "migrate",
		"--project-path", hi.projectPath,
		"--database-dsn", hi.databaseDSN,
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

	readStdout     *os.File
	writeStdout    *os.File
	readStdoutDone chan struct{}

	readStderr     *os.File
	writeStderr    *os.File
	readStderrDone chan struct{}
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

	err = hp.writeStdout.Close()
	assert.NoError(t, err)
	err = hp.writeStderr.Close()
	assert.NoError(t, err)

	select {
	case <-hp.readStdoutDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for to finish logging stdout")
	}

	select {
	case <-hp.readStderrDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for to finish logging stdout")
	}
}

func pipeToLog(t *testing.T, prefix string, r io.Reader) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		t.Log(prefix, s.Text())
	}

	require.NoError(t, s.Err())
}

func spawnHannibal(t *testing.T, args ...string) *hannibalProcess {
	cmdPath := filepath.Join("tmp", "test", "bin", "hannibal")
	argv := []string{cmdPath}
	argv = append(argv, args...)

	readStdout, writeStdout, err := os.Pipe()
	require.NoError(t, err)
	readStdoutDone := make(chan struct{})
	go func() {
		pipeToLog(t, "stdout", readStdout)
		close(readStdoutDone)
	}()
	readStderr, writeStderr, err := os.Pipe()
	require.NoError(t, err)
	readStderrDone := make(chan struct{})
	go func() {
		pipeToLog(t, "stderr", readStderr)
		close(readStderrDone)
	}()

	procAttr := &os.ProcAttr{
		Files: []*os.File{nil, writeStdout, writeStderr},
	}

	process, err := os.StartProcess(cmdPath, argv, procAttr)
	require.NoError(t, err)

	hp := &hannibalProcess{
		process:        process,
		readStdout:     readStdout,
		writeStdout:    writeStdout,
		readStdoutDone: readStdoutDone,
		readStderr:     readStderr,
		writeStderr:    writeStderr,
		readStderrDone: readStderrDone,
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

func (b *browser) post(t *testing.T, queryPath string, contentType string, body []byte) *http.Response {
	response, err := b.client.Post(fmt.Sprintf(`http://%s%s`, b.serverAddr, queryPath), contentType, bytes.NewReader(body))
	require.NoError(t, err)
	return response
}

type apiClient struct {
	serverAddr string
	client     *http.Client
}

func newAPIClient(t *testing.T, serverAddr string) *apiClient {
	client := &http.Client{}
	return &apiClient{
		serverAddr: serverAddr,
		client:     client,
	}
}

func (c *apiClient) get(t *testing.T, queryPath string) *http.Response {
	response, err := c.client.Get(fmt.Sprintf(`http://%s%s`, c.serverAddr, queryPath))
	require.NoError(t, err)
	return response
}

func (c *apiClient) post(t *testing.T, queryPath string, contentType string, body []byte) *http.Response {
	response, err := c.client.Post(fmt.Sprintf(`http://%s%s`, c.serverAddr, queryPath), contentType, bytes.NewReader(body))
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

func runHannibalDevelop(t *testing.T, originalProjectPath string) (*hannibalInstance, func()) {
	appDir := t.TempDir()
	projectPath := filepath.Join(appDir, "project")
	err := copy.Copy(originalProjectPath, projectPath)
	require.NoError(t, err)

	testDB := dbManager.createInitializedDB(t, projectPath)

	hi := &hannibalInstance{
		dbName:      testDB,
		databaseDSN: fmt.Sprintf("database=%s", testDB),
		projectPath: projectPath,
	}

	hi.develop(t)

	return hi, func() {
		hi.stop(t)
		dbManager.dropDB(t, testDB)
	}
}

func runHannibalServe(t *testing.T, projectPath string) (*hannibalInstance, func()) {
	testDB := dbManager.createInitializedDB(t, projectPath)

	appDir := t.TempDir()

	hi := &hannibalInstance{
		dbName:      testDB,
		databaseDSN: fmt.Sprintf("database=%s", testDB),
		appPath:     appDir,
		projectPath: filepath.Join("testdata", "testproject"),
	}

	hi.serve(t)

	hi.systemCreateUser(t, "test")
	apiKey := hi.systemCreateAPIKey(t, "test")
	deployKey := hi.systemCreateDeployKey(t, "test")
	hi.deploy(t, apiKey, deployKey)

	return hi, func() {
		hi.stop(t)
		dbManager.dropDB(t, testDB)
	}
}

func TestMigrate(t *testing.T) {
	t.Parallel()

	testDB := dbManager.createEmptyDB(t)
	defer dbManager.dropDB(t, testDB)

	execHannibal(t,
		"db", "init",
		"--database-dsn", fmt.Sprintf("database=%s", testDB),
	)

	appDir := t.TempDir()
	projectPath := filepath.Join(appDir, "project")
	err := copy.Copy(filepath.Join("testdata", "testproject"), projectPath)
	require.NoError(t, err)

	hi := &hannibalInstance{
		dbName:      testDB,
		databaseDSN: fmt.Sprintf("database=%s", testDB),
		projectPath: projectPath,
	}

	hi.migrate(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, fmt.Sprintf("database=%s", testDB))
	require.NoError(t, err)
	defer conn.Close(ctx)

	// Test that the migration executed.
	var n int
	err = conn.QueryRow(ctx, "select count(*) from todos").Scan(&n)
	require.NoError(t, err)
	assert.EqualValues(t, 0, n)
}

func TestDevelopAutoLoads(t *testing.T) {
	t.Parallel()

	hi, cleanup := runHannibalDevelop(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

	browser := newBrowser(t, hi.httpAddr)
	response := browser.get(t, `/hello?name=Jack`)
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	responseBody := string(readResponseBody(t, response))
	assert.Contains(t, responseBody, "Hello, Jack!")
}

func TestDevelopAutoReloadsOnSQLChanges(t *testing.T) {
	hi, cleanup := runHannibalDevelop(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

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

	err := ioutil.WriteFile(filepath.Join(hi.projectPath, "sql", "hello.sql"), []byte(newSQL), 0644)
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
	t.Parallel()

	hi, cleanup := runHannibalDevelop(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

	newTemplate := `<html>
	<body>
		<p>
			Hello, Alice!
		</p>
	</body>
	</html>
	`

	err := ioutil.WriteFile(filepath.Join(hi.projectPath, "template", "hello.html"), []byte(newTemplate), 0644)
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
	t.Parallel()

	hi, cleanup := runHannibalDevelop(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

	browser := newBrowser(t, hi.httpAddr)
	response := browser.get(t, "/hello.html")
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	responseBody := readResponseBody(t, response)
	fileBody, err := ioutil.ReadFile(filepath.Join("testdata", "testproject", "public", "hello.html"))
	require.NoError(t, err)
	require.Equal(t, fileBody, responseBody)
}

func TestServePublicFiles(t *testing.T) {
	t.Parallel()

	hi, cleanup := runHannibalServe(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

	browser := newBrowser(t, hi.httpAddr)
	response := browser.get(t, "/hello.html")
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	responseBody := readResponseBody(t, response)
	fileBody, err := ioutil.ReadFile(filepath.Join("testdata", "testproject", "public", "hello.html"))
	require.NoError(t, err)
	require.Equal(t, fileBody, responseBody)
}

func TestRouteArgs(t *testing.T) {
	t.Parallel()

	hi, cleanup := runHannibalServe(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

	browser := newBrowser(t, hi.httpAddr)
	response := browser.get(t, "/hello/route/param/Jack")
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	responseBody := string(readResponseBody(t, response))
	assert.Contains(t, responseBody, "Hello, Jack")
}

func TestQueryArgs(t *testing.T) {
	t.Parallel()

	hi, cleanup := runHannibalServe(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

	browser := newBrowser(t, hi.httpAddr)
	response := browser.get(t, "/hello?name=Jack")
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	responseBody := string(readResponseBody(t, response))
	assert.Contains(t, responseBody, "Hello, Jack")
}

func TestFormArgs(t *testing.T) {
	t.Parallel()

	hi, cleanup := runHannibalServe(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

	browser := newBrowser(t, hi.httpAddr)

	response := browser.get(t, "/get_csrf_token")
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	responseBody := string(readResponseBody(t, response))
	match := regexp.MustCompile(`value="(.*)"`).FindStringSubmatch(responseBody)
	require.NotNil(t, match)

	form := url.Values{}
	form.Add("gorilla.csrf.Token", match[1])
	form.Add("name", "Jack")
	response = browser.post(t, "/hello", "application/x-www-form-urlencoded", []byte(form.Encode()))
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	responseBody = string(readResponseBody(t, response))
	assert.Contains(t, responseBody, "Hello, Jack")
}

func TestMethodNotAllowed(t *testing.T) {
	t.Parallel()

	hi, cleanup := runHannibalServe(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

	apiClient := newAPIClient(t, hi.httpAddr)
	response := apiClient.get(t, "/api/hello")
	require.EqualValues(t, http.StatusMethodNotAllowed, response.StatusCode)
}

func TestJSONBodyArgs(t *testing.T) {
	t.Parallel()

	hi, cleanup := runHannibalServe(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

	apiClient := newAPIClient(t, hi.httpAddr)
	response := apiClient.post(t, "/api/hello", "application/json", []byte(`{"name": "Jack"}`))
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
	var responseData map[string]interface{}
	err := json.Unmarshal(readResponseBody(t, response), &responseData)
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"name": "Jack"}, responseData)
}

func TestJSONArrayAndObjectArgs(t *testing.T) {
	t.Parallel()

	hi, cleanup := runHannibalServe(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

	apiClient := newAPIClient(t, hi.httpAddr)
	requestData := map[string]interface{}{
		"untypedArray": []interface{}{"foo", float64(42)},
		"typedArray":   []interface{}{"1", float64(7777)},
		"object": map[string]interface{}{
			"foo": "abc",
			"bar": int64(42),
		},
		"arrayOfObject": []map[string]interface{}{
			{
				"foo": "def",
				"bar": int64(1),
			},
			{
				"foo": "ghi",
				"bar": int64(2),
			},
		},
	}
	expectedResult := map[string]interface{}{
		"untypedArray": []interface{}{"foo", float64(42)},
		"typedArray":   []interface{}{float64(1), float64(7777)},
		"object": map[string]interface{}{
			"foo": "abc",
			"bar": float64(42),
		},
		"arrayOfObject": []interface{}{
			map[string]interface{}{
				"foo": "def",
				"bar": float64(1),
			},
			map[string]interface{}{
				"foo": "ghi",
				"bar": float64(2),
			},
		},
	}
	requestBody, err := json.Marshal(requestData)
	require.NoError(t, err)

	response := apiClient.post(t, "/api/arrays_and_objects", "application/json", requestBody)
	assert.EqualValues(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
	var responseData map[string]interface{}
	err = json.Unmarshal(readResponseBody(t, response), &responseData)
	require.NoError(t, err)
	assert.Equal(t, expectedResult, responseData)
}

func TestArgErrors(t *testing.T) {
	t.Parallel()

	hi, cleanup := runHannibalServe(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

	apiClient := newAPIClient(t, hi.httpAddr)
	response := apiClient.post(t, "/api/todos", "application/json", []byte(`{"name": ""}`))
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
	var responseData map[string]interface{}
	err := json.Unmarshal(readResponseBody(t, response), &responseData)
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"error": map[string]interface{}{"name": "missing"}}, responseData)
}

func TestCookieSession(t *testing.T) {
	t.Parallel()

	hi, cleanup := runHannibalServe(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

	browser := newBrowser(t, hi.httpAddr)
	for i := 1; i < 5; i++ {
		response := browser.get(t, "/hello")
		require.EqualValues(t, http.StatusOK, response.StatusCode)
		responseBody := string(readResponseBody(t, response))
		assert.Contains(t, responseBody, fmt.Sprintf("%d times", i))
	}
}

func TestPasswordDigest(t *testing.T) {
	t.Parallel()

	hi, cleanup := runHannibalServe(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

	apiClient := newAPIClient(t, hi.httpAddr)

	response := apiClient.post(t, "/api/user/login", "application/json", []byte(`{"username": "jack", "password": "secret"}`))
	require.EqualValues(t, http.StatusBadRequest, response.StatusCode)

	response = apiClient.post(t, "/api/user/register", "application/json", []byte(`{"username": "jack", "password": "secret"}`))
	require.EqualValues(t, http.StatusOK, response.StatusCode)

	response = apiClient.post(t, "/api/user/login", "application/json", []byte(`{"username": "jack", "password": "secret"}`))
	require.EqualValues(t, http.StatusOK, response.StatusCode)

	response = apiClient.post(t, "/api/user/login", "application/json", []byte(`{"username": "jack", "password": "wrong"}`))
	require.EqualValues(t, http.StatusBadRequest, response.StatusCode)
}

func TestHTTPResponseHeaders(t *testing.T) {
	t.Parallel()

	hi, cleanup := runHannibalServe(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

	apiClient := newAPIClient(t, hi.httpAddr)
	response := apiClient.get(t, "/response_headers")
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "bar", response.Header.Get("foo"))
}

func TestCSRFProtection(t *testing.T) {
	t.Parallel()

	hi, cleanup := runHannibalServe(t, filepath.Join("testdata", "testproject"))
	defer cleanup()

	browser := newBrowser(t, hi.httpAddr)
	response := browser.post(t, "/csrf_protection_disabled", "application/json", []byte(`{}`))
	require.EqualValues(t, http.StatusOK, response.StatusCode)

	browser = newBrowser(t, hi.httpAddr)
	response = browser.post(t, "/csrf_protection_enabled", "application/json", []byte(`{}`))
	require.EqualValues(t, http.StatusForbidden, response.StatusCode)
	responseBody := string(readResponseBody(t, response))
	assert.Contains(t, responseBody, "Custom CRSF failure message.")

	response = browser.get(t, "/get_csrf_token")
	require.EqualValues(t, http.StatusOK, response.StatusCode)
	responseBody = string(readResponseBody(t, response))
	match := regexp.MustCompile(`value="(.*)"`).FindStringSubmatch(responseBody)
	require.NotNil(t, match)

	form := url.Values{}
	form.Add("gorilla.csrf.Token", match[1])
	response = browser.post(t, "/csrf_protection_enabled", "application/x-www-form-urlencoded", []byte(form.Encode()))
	require.EqualValues(t, http.StatusOK, response.StatusCode)
}
