package peercaller

import (
	"testing"
	"time"

	sdkmodels "github.com/Liapoldus/plugin-sdk/v2/domain/models"
)

func TestSummarizePeerDirectoryIncludesOnlySafeEligibilityMetadata(t *testing.T) {
	until := time.Now().Add(time.Minute)
	directory := sdkmodels.PeerDirectory{Links: []sdkmodels.PeerLink{{
		LinkID: "server-echo",
		Replicas: []sdkmodels.PeerDirectoryReplica{
			{Identity: sdkmodels.PeerReplicaID{PlacementID: "container-echo"}, Eligibility: sdkmodels.PeerEligibilityReady, EligibleUntil: &until},
			{Identity: sdkmodels.PeerReplicaID{PlacementID: "container-echo-backup"}, Eligibility: sdkmodels.PeerEligibilityNotReady},
		},
	}}}

	got := summarizePeerDirectory(directory, "server-echo")
	if !got.LinkFound || got.ReplicaCount != 2 || got.ReadyReplicaCount != 1 {
		t.Fatalf("unexpected summary: %+v", got)
	}
	if len(got.PlacementIDs) != 2 || got.PlacementIDs[0] != "container-echo" || got.PlacementIDs[1] != "container-echo-backup" {
		t.Fatalf("unexpected placement summary: %+v", got.PlacementIDs)
	}
}

func TestSummarizePeerDirectoryDoesNotInventMissingLink(t *testing.T) {
	got := summarizePeerDirectory(sdkmodels.PeerDirectory{}, "server-echo")
	if got.LinkFound || got.ReplicaCount != 0 || got.ReadyReplicaCount != 0 || len(got.PlacementIDs) != 0 {
		t.Fatalf("unexpected missing-link summary: %+v", got)
	}
}
