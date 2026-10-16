package main

import (
	"google.golang.org/protobuf/reflect/protoregistry"
	"testing"
)

// Import the actual executable graph: package-only tests did not load both
// workload services and missed their conflicting generated descriptor paths.
func TestGatewayWorkloadProtocolDescriptorsCoexist(t *testing.T) {
	for _, path := range []string{"gateway_agent_workload.proto", "agent_workload.proto"} {
		if _, err := protoregistry.GlobalFiles.FindFileByPath(path); err != nil {
			t.Fatalf("missing workload protocol %s: %v", path, err)
		}
	}
}
