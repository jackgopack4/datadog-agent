// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package net

import (
	"errors"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
)

const (
	connectionsURL = "http://localhost:3333/" + string(sysconfig.NetworkTracerModule) + "/connections"
	networkIDURL   = "http://unix/" + string(sysconfig.NetworkTracerModule) + "/network_id"
	registerURL    = "http://localhost:3333/" + string(sysconfig.NetworkTracerModule) + "/register"
	statsURL       = "http://localhost:3333/debug/stats"
	tracerouteURL  = "http://localhost:3333/" + string(sysconfig.TracerouteModule) + "/traceroute/"

	// procStatsURL is not used in windows, the value is added to avoid compilation error in windows
	procStatsURL = "http://localhost:3333/" + string(sysconfig.ProcessModule) + "stats"
	// pingURL is not used in windows, the value is added to avoid compilation error in windows
	pingURL = "http://localhost:3333/" + string(sysconfig.PingModule) + "/ping/"

	// systemProbeMaxIdleConns sets the maximum number of idle named pipe connections.
	systemProbeMaxIdleConns = 2

	// systemProbeIdleConnTimeout is the time a named pipe connection is held up idle before being closed.
	// This should be small since connections are local, to close them as soon as they are done,
	// and to quickly service new pending connections.
	systemProbeIdleConnTimeout = 5 * time.Second
)

// CheckPath is used to make sure the globalSocketPath has been set before attempting to connect
func CheckPath(path string) error {
	if path == "" {
		return errors.New("socket path is empty")
	}
	return nil
}

// newSystemProbe creates a group of clients to interact with system-probe.
func newSystemProbe(path string) *RemoteSysProbeUtil {
	return &RemoteSysProbeUtil{
		path:       path,
		httpClient: *client.Get(path),
		tracerouteClient: http.Client{
			// no timeout set here, the expected usage of this client
			// is that the caller will set a timeout on each request
			Transport: &http.Transport{
				MaxIdleConns:    systemProbeMaxIdleConns,
				IdleConnTimeout: systemProbeIdleConnTimeout,
				DialContext:     client.DialContextFunc(path),
			},
		},
	}
}
