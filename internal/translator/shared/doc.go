// Package shared provides common utilities, indexes, limits, metrics, timeouts,
// and status helpers used by the translator package and its sub-packages.
//
// This package is a leaf — it imports only ir, prometheus, and the standard
// library. It does not import any translator internals, keeping it safe to
// use from any sub-package.
package shared
