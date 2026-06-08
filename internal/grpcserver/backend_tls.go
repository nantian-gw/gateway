package grpcserver

import (
	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
	"github.com/nantian-gw/gateway/internal/ir"
)

func toProtoBackendTLS(item *ir.BackendTLSConfig) *controlv1.BackendTlsConfig {
	if item == nil {
		return nil
	}

	return &controlv1.BackendTlsConfig{
		ClientCertificateRef: item.ClientCertificateRef,
	}
}

func toProtoBackendTLSValidation(item *ir.BackendTLSValidation) *controlv1.BackendTlsValidation {
	if item == nil {
		return nil
	}

	return &controlv1.BackendTlsValidation{
		Hostname:                item.Hostname,
		UseSystemCaCertificates: item.UseSystemCAs,
		CaPems:                  append([]string(nil), item.CAPEMs...),
		SubjectAltNames:         toProtoBackendTLSSANs(item.SubjectAltNames),
		MinVersion:              item.MinVersion,
		MaxVersion:              item.MaxVersion,
	}
}

func toProtoBackendTLSSANs(items []ir.BackendSubjectName) []*controlv1.BackendTlsSubjectAltName {
	out := make([]*controlv1.BackendTlsSubjectAltName, 0, len(items))
	for _, item := range items {
		sanType := controlv1.BackendTlsSubjectAltNameType_BACKEND_TLS_SUBJECT_ALT_NAME_TYPE_UNSPECIFIED
		switch item.Type {
		case "Hostname":
			sanType = controlv1.BackendTlsSubjectAltNameType_BACKEND_TLS_SUBJECT_ALT_NAME_TYPE_HOSTNAME
		case "URI":
			sanType = controlv1.BackendTlsSubjectAltNameType_BACKEND_TLS_SUBJECT_ALT_NAME_TYPE_URI
		}
		out = append(out, &controlv1.BackendTlsSubjectAltName{
			Type:  sanType,
			Value: item.Value,
		})
	}
	return out
}
