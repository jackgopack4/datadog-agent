// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package impl implements the healthprobe component interface
package healthprobeimpl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	healthprobeComponent "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

func TestServer(t *testing.T) {

	lc := compdef.NewTestLifecycle(t)
	logComponent := logmock.New(t)

	requires := Requires{
		Lc:  lc,
		Log: logComponent,
		Options: healthprobeComponent.Options{
			Port: 7869,
		},
	}

	provides, err := NewComponent(requires)

	assert.NoError(t, err)

	assert.NotNil(t, provides.Comp)

	ctx := context.Background()

	lc.AssertHooksNumber(1)
	assert.NoError(t, lc.Start(ctx))
	assert.NoError(t, lc.Stop(ctx))
}

func TestServerNoHealthPort(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	logComponent := logmock.New(t)

	requires := Requires{
		Lc:  lc,
		Log: logComponent,
		Options: healthprobeComponent.Options{
			Port: 0,
		},
	}

	provides, err := NewComponent(requires)

	assert.NoError(t, err)

	assert.Nil(t, provides.Comp)
}

func TestLiveHandler(t *testing.T) {
	logComponent := logmock.New(t)

	request := httptest.NewRequest(http.MethodGet, "/live", nil)
	responseRecorder := httptest.NewRecorder()

	liveHandler{logsGoroutines: false, log: logComponent}.ServeHTTP(responseRecorder, request)

	assert.Equal(t, http.StatusOK, responseRecorder.Code)

	assert.Equal(t, "{\"Healthy\":null,\"Unhealthy\":null}", responseRecorder.Body.String())
}

func TestLiveHandlerUnhealthy(t *testing.T) {
	logComponent := logmock.New(t)

	request := httptest.NewRequest(http.MethodGet, "/live", nil)
	responseRecorder := httptest.NewRecorder()

	handler := health.RegisterLiveness("fake")
	defer func() {
		health.Deregister(handler)
	}()

	liveHandler{logsGoroutines: false, log: logComponent}.ServeHTTP(responseRecorder, request)

	assert.Equal(t, http.StatusInternalServerError, responseRecorder.Code)

	assert.Equal(t, "{\"Healthy\":[\"healthcheck\"],\"Unhealthy\":[\"fake\"]}", responseRecorder.Body.String())
}

func TestReadyHandler(t *testing.T) {
	logComponent := logmock.New(t)

	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	responseRecorder := httptest.NewRecorder()

	readyHandler{logsGoroutines: false, log: logComponent}.ServeHTTP(responseRecorder, request)

	assert.Equal(t, http.StatusOK, responseRecorder.Code)

	assert.Equal(t, "{\"Healthy\":null,\"Unhealthy\":null}", responseRecorder.Body.String())
}

func TestReadyHandlerUnhealthy(t *testing.T) {
	logComponent := logmock.New(t)

	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	responseRecorder := httptest.NewRecorder()

	handler := health.RegisterReadiness("fake")
	defer func() {
		health.Deregister(handler)
	}()

	readyHandler{logsGoroutines: false, log: logComponent}.ServeHTTP(responseRecorder, request)

	assert.Equal(t, http.StatusInternalServerError, responseRecorder.Code)

	assert.Equal(t, "{\"Healthy\":[\"healthcheck\"],\"Unhealthy\":[\"fake\"]}", responseRecorder.Body.String())
}

func TestHealthHandlerFails(t *testing.T) {
	logComponent := logmock.New(t)

	request := httptest.NewRequest(http.MethodGet, "/live", nil)
	responseRecorder := httptest.NewRecorder()

	healthHandler(false, logComponent, func() (health.Status, error) {
		return health.Status{}, fmt.Errorf("fail to extract status")
	}, responseRecorder, request)

	assert.Equal(t, http.StatusInternalServerError, responseRecorder.Code)
	assert.Equal(t, "{\"error\":\"fail to extract status\"}\n", responseRecorder.Body.String())
}
