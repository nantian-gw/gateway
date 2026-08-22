package admin

import (
	"bytes"
	"context"
	"errors"
	jsoniter "github.com/json-iterator/go"
	"net/http"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var bufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

var slicePool = sync.Pool{
	New: func() any { s := make([]byte, 0, 4096); return &s },
}

func (s *Server) respondJSON(w http.ResponseWriter, payload any) {
	if payload == nil {
		payload = map[string]any{}
	}

	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	buffer := newLimitedBuffer(s.maxResponseBodyBytes, errPayloadTooLarge("response exceeds admin response size limit"))
	buffer.buf = *buf
	if err := jsoniter.NewEncoder(buffer).Encode(payload); err != nil {
		if isPayloadTooLarge(err) {
			s.respondRequestError(w, err)
			return
		}
		s.logger.Error("failed to encode admin response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(buffer.Bytes()); err != nil {
		s.logger.Error("failed to write admin response", "error", err)
	}
}

func (s *Server) respondQueryError(w http.ResponseWriter, err error) {
	if isInvalidQuery(err) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Error(w, err.Error(), statusCodeForAdminError(err))
}

func (s *Server) respondRequestError(w http.ResponseWriter, err error) {
	switch {
	case isInvalidRequest(err):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case isPayloadTooLarge(err):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
	case errors.Is(err, context.Canceled):
		http.Error(w, err.Error(), http.StatusRequestTimeout)
	default:
		http.Error(w, err.Error(), statusCodeForAdminError(err))
	}
}

type limitedBuffer struct {
	limit int64
	buf   bytes.Buffer
	err   error
}

func newLimitedBuffer(limit int64, err error) *limitedBuffer {
	return &limitedBuffer{limit: limit, err: err}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if int64(len(p)) > b.limit-int64(b.buf.Len()) {
		return 0, b.err
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}

func (s *Server) respondNotFound(w http.ResponseWriter, message string) {
	http.Error(w, message, http.StatusNotFound)
}

func statusCodeForAdminError(err error) int {
	switch {
	case apierrors.IsNotFound(err):
		return http.StatusNotFound
	case apierrors.IsAlreadyExists(err):
		return http.StatusConflict
	case apierrors.IsConflict(err):
		return http.StatusConflict
	case apierrors.IsForbidden(err):
		return http.StatusForbidden
	case apierrors.IsUnauthorized(err):
		return http.StatusUnauthorized
	case apierrors.IsTooManyRequests(err):
		return http.StatusTooManyRequests
	case apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return http.StatusGatewayTimeout
	case apierrors.IsServiceUnavailable(err):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
