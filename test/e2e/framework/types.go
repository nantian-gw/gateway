//go:build e2e

package framework

type T interface {
	Helper()
	Log(args ...interface{})
	Logf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
}
