// Package stationidentity contains pure validation used by the future
// station-api. It does not contact Kubernetes and is not an authentication
// implementation by itself.
package stationidentity

import (
	"crypto/subtle"
	"errors"
)

type Evidence struct {
	ClusterID, RegisteredClusterID                                                             string
	ServiceAccountNamespace, ServiceAccountName, RegisteredNamespace, RegisteredServiceAccount string
	TokenServiceAccountUID, RegisteredServiceAccountUID, PodServiceAccountName                 string
	TokenPodName, PodName, PodNodeName, NodeName                                               string
	TokenPodUID, PodUID, NodeUID, RegisteredNodeUID, StationUID, RegisteredStationUID          string
	TokenAudiences, RequiredAudiences                                                          []string
	TokenAuthenticated, PodDeleted, NodeDeleted, BindingInactive                               bool
	Generation, RegisteredGeneration                                                           int64
	// NodePressures are the pressure conditions that are True on the Node the
	// identity was verified against. They are no part of the identity.
	NodePressures []string
}

func Validate(e Evidence) error {
	if !e.TokenAuthenticated {
		return errors.New("token is not authenticated")
	}
	if e.ClusterID == "" || e.ClusterID != e.RegisteredClusterID {
		return errors.New("cluster identity mismatch")
	}
	if e.ServiceAccountNamespace == "" || e.ServiceAccountNamespace != e.RegisteredNamespace || e.ServiceAccountName == "" || e.ServiceAccountName != e.RegisteredServiceAccount {
		return errors.New("service account identity mismatch")
	}
	if e.TokenServiceAccountUID == "" || e.TokenServiceAccountUID != e.RegisteredServiceAccountUID || e.PodServiceAccountName != e.ServiceAccountName {
		return errors.New("service account instance mismatch")
	}
	if e.TokenPodName == "" || e.TokenPodName != e.PodName || e.TokenPodUID == "" || e.TokenPodUID != e.PodUID || e.PodDeleted || e.NodeDeleted {
		return errors.New("pod identity is not current")
	}
	if e.PodNodeName == "" || e.PodNodeName != e.NodeName || e.NodeUID == "" || e.NodeUID != e.RegisteredNodeUID || e.BindingInactive {
		return errors.New("node binding mismatch")
	}
	stationUnbound := e.StationUID == "" && e.RegisteredStationUID == "" && e.Generation == 0 && e.RegisteredGeneration == 0
	stationBound := e.StationUID != "" && e.StationUID == e.RegisteredStationUID && e.Generation > 0 && e.Generation == e.RegisteredGeneration
	if !stationUnbound && !stationBound {
		return errors.New("station binding mismatch")
	}
	if len(e.RequiredAudiences) == 0 || !audienceContainsAll(e.TokenAudiences, e.RequiredAudiences) {
		return errors.New("token audience is not authorized")
	}
	return nil
}

func audienceContainsAll(got, required []string) bool {
	for _, want := range required {
		if want == "" {
			return false
		}
		found := false
		for _, value := range got {
			if subtle.ConstantTimeCompare([]byte(value), []byte(want)) == 1 {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
