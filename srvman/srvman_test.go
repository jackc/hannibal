package srvman_test

import (
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/hannibal/srvman"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestLogWriter struct {
	T *testing.T
}

func (w *TestLogWriter) Write(p []byte) (int, error) {
	w.T.Log(string(p))
	return len(p), nil
}

func TestGroupSimpleLifeCycle(t *testing.T) {

	makeGroup := func() *srvman.Group {
		tlr := &TestLogWriter{T: t}
		output := zerolog.ConsoleWriter{Out: tlr}
		logger := zerolog.New(output)

		return &srvman.Group{
			ServiceConfigs: []*srvman.ServiceConfig{
				{
					Name:        "http_hello",
					Cmd:         "tmp/test/bin/http_server",
					Args:        []string{"[[bluegreen.port]]"},
					HTTPAddress: "http://127.0.0.1:[[bluegreen.port]]",
					HealthCheck: &srvman.HealthCheck{
						TCPConnect: "127.0.0.1:[[bluegreen.port]]",
					},
					MaxStartupDuration: 5 * time.Second,
					Blue:               map[string]interface{}{"port": 4200},
					Green:              map[string]interface{}{"port": 4201},
					Logger:             &logger,
				},
			},
		}
	}

	blueGroup := makeGroup()

	err := blueGroup.Start(srvman.ColorBlue)
	require.NoError(t, err)
	blueStopped := false
	defer func() {
		if !blueStopped {
			blueGroup.Stop()
		}
	}()

	resp, err := http.Get("http://127.0.0.1:4200/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", string(body))

	greenGroup := makeGroup()

	err = greenGroup.Start(srvman.ColorGreen)
	require.NoError(t, err)
	greenStopped := false
	defer func() {
		if !greenStopped {
			greenGroup.Stop()
		}
	}()

	err = blueGroup.Stop()
	blueStopped = true
	require.NoError(t, err)

	resp, err = http.Get("http://127.0.0.1:4201/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", string(body))

	err = greenGroup.Stop()
	greenStopped = true
	require.NoError(t, err)
}
