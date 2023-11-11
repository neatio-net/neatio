package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/docker/docker/pkg/reexec"
	"github.com/neatio-net/neatio/internal/cmdtest"
)

func tmpdir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "neatio-test")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

type testneatchain struct {
	*cmdtest.TestCmd

	Datadir   string
	Etherbase string
}

func init() {

	reexec.Register("neatio-test", func() {
		if err := app.Run(os.Args); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	})
}

func TestMain(m *testing.M) {

	if reexec.Init() {
		return
	}
	os.Exit(m.Run())
}

func runneatchain(t *testing.T, args ...string) *testneatchain {
	tt := &testneatchain{}
	tt.TestCmd = cmdtest.NewTestCmd(t, tt)
	for i, arg := range args {
		switch {
		case arg == "-datadir" || arg == "--datadir":
			if i < len(args)-1 {
				tt.Datadir = args[i+1]
			}
		case arg == "-etherbase" || arg == "--etherbase":
			if i < len(args)-1 {
				tt.Etherbase = args[i+1]
			}
		}
	}
	if tt.Datadir == "" {
		tt.Datadir = tmpdir(t)
		tt.Cleanup = func() { os.RemoveAll(tt.Datadir) }
		args = append([]string{"-datadir", tt.Datadir}, args...)

		defer func() {
			if t.Failed() {
				tt.Cleanup()
			}
		}()
	}

	tt.Run("neatio-test", args...)

	return tt
}
