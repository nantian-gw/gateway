package grpcserver

import (
	controlv1 "github.com/aether-gateway/proto/gateway/control/v1"
	"github.com/aether-gateway/aether-gateway/controlplane/internal/ir"
)

func toProtoFrontendValidation(item *ir.FrontendValidation) *controlv1.FrontendValidation {
	if item == nil || (len(item.ClientCAPEMs) == 0 && item.Mode == "") {
		return nil
	}

	return &controlv1.FrontendValidation{
		CaPems: item.ClientCAPEMs,
		Mode:   item.Mode,
	}
}
