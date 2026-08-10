package connector

import (
	"errors"
	"strings"
	"testing"

	"github.com/intruder0007/Lumo/core/diag"
)

type fakeConnector struct {
	name   string
	phases []Phase
}

func (f fakeConnector) Name() string    { return f.name }
func (f fakeConnector) Phases() []Phase { return f.phases }

func TestRunEngineRunsPhasesInOrderAndReturnsLastResult(t *testing.T) {
	var order []string
	c := fakeConnector{name: "test", phases: []Phase{
		{Name: "auth", Run: func() (Result, error) { order = append(order, "auth"); return Result{Connected: true}, nil }},
		{Name: "fetch", Run: func() (Result, error) { order = append(order, "fetch"); return Result{Connected: true}, nil }},
		{Name: "validate", Run: func() (Result, error) { order = append(order, "validate"); return Result{Connected: true, Detail: "ok"}, nil }},
	}}

	got, err := RunEngine(c, diag.NoopLogger{})
	if err != nil {
		t.Fatalf("RunEngine: %v", err)
	}
	if !got.Connected || got.Detail != "ok" {
		t.Errorf("RunEngine result = %+v, want Connected=true Detail=%q", got, "ok")
	}
	wantOrder := []string{"auth", "fetch", "validate"}
	if len(order) != len(wantOrder) {
		t.Fatalf("phase order = %v, want %v", order, wantOrder)
	}
	for i, name := range wantOrder {
		if order[i] != name {
			t.Errorf("phase order = %v, want %v", order, wantOrder)
			break
		}
	}
}

func TestRunEngineShortCircuitsOnFailureNamingThePhase(t *testing.T) {
	var ran []string
	c := fakeConnector{name: "sonarqube", phases: []Phase{
		{Name: "auth", Run: func() (Result, error) {
			ran = append(ran, "auth")
			return Result{}, errors.New("token rejected (401)")
		}},
		{Name: "fetch", Run: func() (Result, error) {
			ran = append(ran, "fetch")
			return Result{Connected: true}, nil
		}},
	}}

	_, err := RunEngine(c, diag.NoopLogger{})
	if err == nil {
		t.Fatal("RunEngine: want error, got nil")
	}
	if !strings.Contains(err.Error(), "sonarqube") || !strings.Contains(err.Error(), "auth") || !strings.Contains(err.Error(), "token rejected (401)") {
		t.Errorf("RunEngine error = %q, want it to name the connector, phase, and underlying error", err.Error())
	}
	if len(ran) != 1 || ran[0] != "auth" {
		t.Errorf("phases run = %v, want only [auth] (short-circuit)", ran)
	}
}
