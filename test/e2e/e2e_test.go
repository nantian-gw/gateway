//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"testing"

	"github.com/nantian-gw/gateway/test/e2e/framework"
)

func TestMain(m *testing.M) {
	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		clusterName = "nantian-e2e"
	}
	os.Setenv("CLUSTER_NAME", clusterName)

	skipSetup := os.Getenv("SKIP_SETUP") == "true"
	skipCleanup := os.Getenv("SKIP_CLEANUP") == "true"

	if !skipSetup {
		framework.CreateKindCluster(&testTB{}, clusterName)
		framework.InstallGatewayAPICRDs(&testTB{})
		framework.DeployNantianGW(&testTB{})
	}

	code := m.Run()

	if !skipCleanup {
		framework.DeleteKindCluster(&testTB{}, clusterName)
	}

	os.Exit(code)
}

type testTB struct{}

func (t *testTB) Helper() {}

func (t *testTB) Log(args ...interface{}) {
	fmt.Fprintln(os.Stderr, append([]interface{}{"[e2e-setup]"}, args...)...)
}

func (t *testTB) Logf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[e2e-setup] "+format+"\n", args...)
}

func (t *testTB) Fatal(args ...interface{}) {
	fmt.Fprintln(os.Stderr, append([]interface{}{"[e2e-setup] FATAL:"}, args...)...)
	os.Exit(1)
}

func (t *testTB) Fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[e2e-setup] FATAL: "+format+"\n", args...)
	os.Exit(1)
}

func (t *testTB) Error(args ...interface{}) {
	fmt.Fprintln(os.Stderr, append([]interface{}{"[e2e-setup] ERROR:"}, args...)...)
}

func (t *testTB) Errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[e2e-setup] ERROR: "+format+"\n", args...)
}
