package main

import (
	"context"
	"testing"
)

func TestMemoryInstanceStoreIsolatedByOwner(t *testing.T) {
	instanceDB = nil
	memoryInstancesMu.Lock()
	memoryInstances = map[string]instance{}
	memoryInstancesMu.Unlock()
	ctx := context.Background()
	first := instance{ID: "one", OwnerID: "user-a", Name: "第一个", Image: "alemonx:test", Version: "test", Spec: "2 核 / 4 GB", Status: "运行中", IP: "专属域名", ContainerName: "xcloud-a1b2c3d4", CreatedAt: "2026-09-04 10:00"}
	second := instance{ID: "two", OwnerID: "user-b", Name: "第二个", Image: "alemonx:test", Version: "test", Spec: "2 核 / 4 GB", Status: "运行中", IP: "专属域名", ContainerName: "xcloud-b1c2d3e4", CreatedAt: "2026-09-04 10:00"}
	if err := saveStoredInstance(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := saveStoredInstance(ctx, second); err != nil {
		t.Fatal(err)
	}
	items, err := listStoredInstances(ctx, "user-a")
	if err != nil || len(items) != 1 || items[0].ID != first.ID {
		t.Fatalf("unexpected owner result: %#v, %v", items, err)
	}
	first.Status = "已停止"
	if err := saveStoredInstance(ctx, first); err != nil {
		t.Fatal(err)
	}
	found, ok, err := getStoredInstance(ctx, first.ID, first.OwnerID)
	if err != nil || !ok || found.Status != "已停止" {
		t.Fatalf("unexpected saved instance: %#v, %t, %v", found, ok, err)
	}
	if err := removeStoredInstance(ctx, first.ID, first.OwnerID); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := getStoredInstance(ctx, first.ID, first.OwnerID); err != nil || ok {
		t.Fatalf("instance should be removed: %t, %v", ok, err)
	}
}
