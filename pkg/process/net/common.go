// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package net

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	netEncoding "github.com/DataDog/datadog-agent/pkg/network/encoding/unmarshal"
	nppayload "github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	procEncoding "github.com/DataDog/datadog-agent/pkg/process/encoding"
	reqEncoding "github.com/DataDog/datadog-agent/pkg/process/encoding/request"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// Conn is a wrapper over some net.Listener
type Conn interface {
	// GetListener returns the underlying net.Listener
	GetListener() net.Listener

	// Stop and clean up resources for the underlying connection
	Stop()
}

const (
	contentTypeProtobuf = "application/protobuf"
	contentTypeJSON     = "application/json"
)

var _ SysProbeUtil = &RemoteSysProbeUtil{}

// RemoteSysProbeUtil wraps interactions with a remote system probe service
type RemoteSysProbeUtil struct {
	// Retrier used to setup system probe
	initRetry retry.Retrier

	path             string
	httpClient       http.Client
	tracerouteClient http.Client
}

// ensure that GetRemoteSystemProbeUtil implements SysProbeUtilGetter
var _ SysProbeUtilGetter = GetRemoteSystemProbeUtil

// GetRemoteSystemProbeUtil returns a ready to use RemoteSysProbeUtil. It is backed by a shared singleton.
func GetRemoteSystemProbeUtil(path string) (SysProbeUtil, error) {
	sysProbeUtil, err := getRemoteSystemProbeUtil(path)
	if err != nil {
		return nil, err
	}

	if err := sysProbeUtil.initRetry.TriggerRetry(); err != nil {
		log.Debugf("system probe init error: %s", err)
		return nil, err
	}

	return sysProbeUtil, nil
}

var getRemoteSystemProbeUtil = funcs.MemoizeArg(func(path string) (*RemoteSysProbeUtil, error) {
	err := CheckPath(path)
	if err != nil {
		return nil, fmt.Errorf("error setting up remote system probe util, %v", err)
	}

	sysProbeUtil := newSystemProbe(path)
	err = sysProbeUtil.initRetry.SetupRetrier(&retry.Config{ //nolint:errcheck
		Name:          "system-probe-util",
		AttemptMethod: sysProbeUtil.init,
		Strategy:      retry.RetryCount,
		// 10 tries w/ 30s delays = 5m of trying before permafail
		RetryCount: 10,
		RetryDelay: 30 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return sysProbeUtil, nil
})

// GetProcStats returns a set of process stats by querying system-probe
func (r *RemoteSysProbeUtil) GetProcStats(pids []int32) (*model.ProcStatsWithPermByPID, error) {
	procReq := &pbgo.ProcessStatRequest{
		Pids: pids,
	}

	reqBody, err := reqEncoding.GetMarshaler(reqEncoding.ContentTypeProtobuf).Marshal(procReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", procStatsURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", contentTypeProtobuf)
	req.Header.Set("Content-Type", procEncoding.ContentTypeProtobuf)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proc_stats request failed: Probe Path %s, url: %s, status code: %d", r.path, procStatsURL, resp.StatusCode)
	}

	body, err := readAllResponseBody(resp)
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-type")
	results, err := procEncoding.GetUnmarshaler(contentType).Unmarshal(body)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// GetConnections returns a set of active network connections, retrieved from the system probe service
func (r *RemoteSysProbeUtil) GetConnections(clientID string) (*model.Connections, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s?client_id=%s", connectionsURL, clientID), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", contentTypeProtobuf)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conn request failed: Probe Path %s, url: %s, status code: %d", r.path, connectionsURL, resp.StatusCode)
	}

	body, err := readAllResponseBody(resp)
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-type")
	conns, err := netEncoding.GetUnmarshaler(contentType).Unmarshal(body)
	if err != nil {
		return nil, err
	}

	return conns, nil
}

// GetNetworkID fetches the network_id (vpc_id) from system-probe
func (r *RemoteSysProbeUtil) GetNetworkID() (string, error) {
	req, err := http.NewRequest("GET", networkIDURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "text/plain")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("network_id request failed: url: %s, status code: %d", networkIDURL, resp.StatusCode)
	}

	body, err := readAllResponseBody(resp)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(body), nil
}

// GetPing returns the results of a ping to a host
func (r *RemoteSysProbeUtil) GetPing(clientID string, host string, count int, interval time.Duration, timeout time.Duration) ([]byte, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s?client_id=%s&count=%d&interval=%d&timeout=%d", pingURL, host, clientID, count, interval, timeout), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", contentTypeJSON)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest {
		body, err := readAllResponseBody(resp)
		if err != nil {
			return nil, fmt.Errorf("ping request failed: Probe Path %s, url: %s, status code: %d", r.path, pingURL, resp.StatusCode)
		}
		return nil, fmt.Errorf("ping request failed: Probe Path %s, url: %s, status code: %d, error: %s", r.path, pingURL, resp.StatusCode, string(body))
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ping request failed: Probe Path %s, url: %s, status code: %d", r.path, pingURL, resp.StatusCode)
	}

	body, err := readAllResponseBody(resp)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// GetTraceroute returns the results of a traceroute to a host
func (r *RemoteSysProbeUtil) GetTraceroute(clientID string, host string, port uint16, protocol nppayload.Protocol, maxTTL uint8, timeout time.Duration) ([]byte, error) {
	httpTimeout := timeout*time.Duration(maxTTL) + 10*time.Second // allow extra time for the system probe communication overhead, calculate full timeout for TCP traceroute
	log.Tracef("Network Path traceroute HTTP request timeout: %s", httpTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/%s?client_id=%s&port=%d&max_ttl=%d&timeout=%d&protocol=%s", tracerouteURL, host, clientID, port, maxTTL, timeout, protocol), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", contentTypeJSON)
	resp, err := r.tracerouteClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest {
		body, err := readAllResponseBody(resp)
		if err != nil {
			return nil, fmt.Errorf("traceroute request failed: Probe Path %s, url: %s, status code: %d", r.path, tracerouteURL, resp.StatusCode)
		}
		return nil, fmt.Errorf("traceroute request failed: Probe Path %s, url: %s, status code: %d, error: %s", r.path, tracerouteURL, resp.StatusCode, string(body))
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("traceroute request failed: Probe Path %s, url: %s, status code: %d", r.path, tracerouteURL, resp.StatusCode)
	}

	body, err := readAllResponseBody(resp)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// Register registers the client to system probe
func (r *RemoteSysProbeUtil) Register(clientID string) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s?client_id=%s", registerURL, clientID), nil)
	if err != nil {
		return err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("conn request failed: Path %s, url: %s, status code: %d", r.path, statsURL, resp.StatusCode)
	}

	return nil
}

func (r *RemoteSysProbeUtil) init() error {
	resp, err := r.httpClient.Get(statsURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote tracer status check failed: socket %s, url: %s, status code: %d", r.path, statsURL, resp.StatusCode)
	}
	return nil
}

func readAllResponseBody(resp *http.Response) ([]byte, error) {
	// if we are not able to determine the content length
	// we read the whole body without pre-allocation
	if resp.ContentLength <= 0 {
		return io.ReadAll(resp.Body)
	}

	// if we know the content length we pre-allocate the buffer
	var buf bytes.Buffer
	buf.Grow(int(resp.ContentLength))

	_, err := buf.ReadFrom(resp.Body)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
