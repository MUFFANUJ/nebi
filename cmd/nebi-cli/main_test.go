package main

import "testing"

func TestRootCommandDoesNotExposeServe(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "serve" {
			t.Fatal("nebi-cli must not expose the serve subcommand")
		}
	}
}
