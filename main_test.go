package main

import (
	"path/filepath"
	"testing"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		args    []string
		want    options
		wantErr bool
	}{
		{nil, options{}, false},
		{[]string{"--help"}, options{help: true}, false},
		{[]string{"-h"}, options{help: true}, false},
		{[]string{"--version"}, options{version: true}, false},
		{[]string{"-v"}, options{version: true}, false},
		{[]string{"--debug"}, options{debug: true}, false},
		{[]string{"--bogus"}, options{}, true},
		{[]string{"extra"}, options{}, true},
	}
	for _, c := range cases {
		got, err := parseArgs(c.args)
		if (err != nil) != c.wantErr {
			t.Errorf("parseArgs(%v) error = %v, wantErr %v", c.args, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("parseArgs(%v) = %+v, want %+v", c.args, got, c.want)
		}
	}
}

func TestDebugLogPathUsesXDGStateHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	got, err := debugLogPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "noscli", "debug.log"); got != want {
		t.Errorf("debugLogPath() = %q, want %q", got, want)
	}
}
