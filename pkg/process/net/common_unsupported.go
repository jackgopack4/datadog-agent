// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

package net

import (
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	nppayload "github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

var _ SysProbeUtil = &RemoteSysProbeUtil{}
var _ SysProbeUtilGetter = GetRemoteSystemProbeUtil

// RemoteSysProbeUtil is not supported
type RemoteSysProbeUtil struct{}

// CheckPath is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func CheckPath(_ string) error {
	return ErrNotImplemented
}

// GetRemoteSystemProbeUtil is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func GetRemoteSystemProbeUtil(_ string) (SysProbeUtil, error) {
	return &RemoteSysProbeUtil{}, ErrNotImplemented
}

// GetConnections is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func (r *RemoteSysProbeUtil) GetConnections(_ string) (*model.Connections, error) {
	return nil, ErrNotImplemented
}

// GetNetworkID is not supported
func (r *RemoteSysProbeUtil) GetNetworkID() (string, error) {
	return "", ErrNotImplemented
}

// GetProcStats is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func (r *RemoteSysProbeUtil) GetProcStats(_ []int32) (*model.ProcStatsWithPermByPID, error) {
	return nil, ErrNotImplemented
}

// Register is not supported
//
//nolint:revive // TODO(PROC) Fix revive linter
func (r *RemoteSysProbeUtil) Register(_ string) error {
	return ErrNotImplemented
}

// GetPing is not supported
func (r *RemoteSysProbeUtil) GetPing(_ string, _ string, _ int, _ time.Duration, _ time.Duration) ([]byte, error) {
	return nil, ErrNotImplemented
}

// GetTraceroute is not supported
func (r *RemoteSysProbeUtil) GetTraceroute(_ string, _ string, _ uint16, _ nppayload.Protocol, _ uint8, _ time.Duration) ([]byte, error) {
	return nil, ErrNotImplemented
}
