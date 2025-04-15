package cmd

import "testing"

func TestEcho(t *testing.T) {
	output := Echo("Dog")
	if output != "Greeting, Dog" {
		t.Fail()
	}
}
