package translator

import "github.com/nantian-gw/gateway/internal/translator/policies"

// SetupIndexes is an alias kept for backward compatibility with callers that
// reference translator.SetupIndexes (e.g. cmd/manager).
var SetupIndexes = policies.SetupIndexes
