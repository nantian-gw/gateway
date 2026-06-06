package admin

import "errors"

type invalidQueryError struct {
	message string
}

func (e invalidQueryError) Error() string {
	return e.message
}

func errInvalidQuery(message string) error {
	return invalidQueryError{message: message}
}

func isInvalidQuery(err error) bool {
	var target invalidQueryError
	return errors.As(err, &target)
}

type invalidRequestError struct {
	message string
}

func (e invalidRequestError) Error() string {
	return e.message
}

func errInvalidRequest(message string) error {
	return invalidRequestError{message: message}
}

func isInvalidRequest(err error) bool {
	var target invalidRequestError
	return errors.As(err, &target)
}

type payloadTooLargeError struct {
	message string
}

func (e payloadTooLargeError) Error() string {
	return e.message
}

func errPayloadTooLarge(message string) error {
	return payloadTooLargeError{message: message}
}

func isPayloadTooLarge(err error) bool {
	var target payloadTooLargeError
	return errors.As(err, &target)
}

type serviceUnavailableError struct {
	message string
}

func (e serviceUnavailableError) Error() string {
	return e.message
}

func errServiceUnavailable(message string) error {
	return serviceUnavailableError{message: message}
}
