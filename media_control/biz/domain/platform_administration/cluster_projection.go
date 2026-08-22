package platform_administration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

var clusterIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

type ClusterStatus string

const (
	ClusterOnline   ClusterStatus = "online"
	ClusterDegraded ClusterStatus = "degraded"
	ClusterOffline  ClusterStatus = "offline"
	ClusterUnknown  ClusterStatus = "unknown"
)

type ClusterProjection struct {
	ClusterID, CapacityJSON string
	Status                  ClusterStatus
	ObservedAt              time.Time
}

type ClusterRepository interface {
	UpsertCluster(context.Context, *ClusterProjection) error
	ListClusters(context.Context) ([]ClusterProjection, error)
	GetCluster(context.Context, string) (*ClusterProjection, error)
}

func ValidateCluster(clusterID, status, capacityJSON string, observedAt time.Time) (string, ClusterStatus, string, error) {
	clusterID, status, capacityJSON = strings.TrimSpace(clusterID), strings.TrimSpace(status), strings.TrimSpace(capacityJSON)
	if !clusterIDPattern.MatchString(clusterID) {
		return "", "", "", errors.New("cluster_id is invalid")
	}
	parsedStatus := ClusterStatus(status)
	switch parsedStatus {
	case ClusterOnline, ClusterDegraded, ClusterOffline, ClusterUnknown:
	default:
		return "", "", "", errors.New("status must be online, degraded, offline or unknown")
	}
	if observedAt.IsZero() {
		return "", "", "", errors.New("observed_at must be positive")
	}
	if capacityJSON == "" || len(capacityJSON) > 64*1024 {
		return "", "", "", errors.New("capacity_json must be a JSON object no larger than 64 KiB")
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(capacityJSON), &object); err != nil || object == nil {
		return "", "", "", errors.New("capacity_json must be a JSON object no larger than 64 KiB")
	}
	var compact bytes.Buffer
	_ = json.Compact(&compact, []byte(capacityJSON))
	return clusterID, parsedStatus, compact.String(), nil
}
