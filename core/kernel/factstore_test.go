package kernel

import "testing"

func TestFactStore_GetUnknownDomain(t *testing.T) {
	f := NewFactStore()
	if _, ok := f.Get("workspace-intelligence"); ok {
		t.Fatal("Get on a domain that never published should return ok=false")
	}
}

func TestFactStore_PublishThenGet(t *testing.T) {
	f := NewFactStore()
	type stubModel struct{ Value int }

	f.Publish("workspace-intelligence", stubModel{Value: 1})
	got, ok := f.Get("workspace-intelligence")
	if !ok {
		t.Fatal("Get after Publish should return ok=true")
	}
	model, ok := got.(stubModel)
	if !ok || model.Value != 1 {
		t.Fatalf("Get returned %#v, want stubModel{Value: 1}", got)
	}
}

func TestFactStore_PublishOverwritesWholeModel(t *testing.T) {
	f := NewFactStore()
	type stubModel struct{ Value int }

	f.Publish("workspace-intelligence", stubModel{Value: 1})
	f.Publish("workspace-intelligence", stubModel{Value: 2})

	got, _ := f.Get("workspace-intelligence")
	if model := got.(stubModel); model.Value != 2 {
		t.Fatalf("Get after second Publish = %#v, want stubModel{Value: 2}", model)
	}
}

func TestFactStore_DomainsAreIndependent(t *testing.T) {
	f := NewFactStore()
	f.Publish("workspace-intelligence", "ws-model")
	f.Publish("source-control", "scm-model")

	if got, _ := f.Get("workspace-intelligence"); got != "ws-model" {
		t.Fatalf("workspace-intelligence model = %v, want ws-model", got)
	}
	if got, _ := f.Get("source-control"); got != "scm-model" {
		t.Fatalf("source-control model = %v, want scm-model", got)
	}
}

func TestFactStore_ConcurrentAccess(t *testing.T) {
	f := NewFactStore()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			f.Publish("workspace-intelligence", i)
		}
		close(done)
	}()
	for i := 0; i < 1000; i++ {
		f.Get("workspace-intelligence")
	}
	<-done
}
