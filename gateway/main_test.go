package main

import "testing"

func TestGatewayClusterModeDefaultsToSingle(t *testing.T) {
	t.Setenv("GATEWAY_MODE", "")
	cluster, err := gatewayClusterMode()
	if err != nil || cluster {
		t.Fatalf("single mode = %v, %v", cluster, err)
	}
}

func TestGatewayClusterModeAcceptsClusterOnly(t *testing.T) {
	t.Setenv("GATEWAY_MODE", "cluster")
	cluster, err := gatewayClusterMode()
	if err != nil || !cluster {
		t.Fatalf("cluster mode = %v, %v", cluster, err)
	}
	t.Setenv("GATEWAY_MODE", "unknown")
	if _, err = gatewayClusterMode(); err == nil {
		t.Fatal("expected invalid mode rejection")
	}
}
