package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/MimeLyc/contextual-sub-translator/pkg/log"
)

type ErrorType int

const (
	ErrFileNotFound ErrorType = iota
	ErrFileRead
	ErrFileWrite
	ErrParse
	ErrAPI
	ErrValidation
	ErrConfig
	ErrNetwork
	ErrTranslation
	ErrUnknown
)

type CTXTransError struct {
	Type    ErrorType
	Message string
	Context map[string]any
	Cause   error
}

func NewError(errorType ErrorType, message string) *CTXTransError {
	return &CTXTransError{
		Type:    errorType,
		Message: message,
		Context: make(map[string]any),
	}
}

func NewErrorWithCause(errorType ErrorType, message string, cause error) *CTXTransError {
	return &CTXTransError{
		Type:    errorType,
		Message: message,
		Context: make(map[string]any),
		Cause:   cause,
	}
}

func (e *CTXTransError) Error() string {
	var parts []string
	parts = append(parts, fmt.Sprintf("[%s] %s", e.Type.String(), e.Message))

	if len(e.Context) > 0 {
		var ctxParts []string
		for k, v := range e.Context {
			ctxParts = append(ctxParts, fmt.Sprintf("%s=%v", k, v))
		}
		parts = append(parts, fmt.Sprintf("context: %s", strings.Join(ctxParts, ", ")))
	}

	if e.Cause != nil {
		parts = append(parts, fmt.Sprintf("cause: %v", e.Cause))
	}

	return strings.Join(parts, " | ")
}

func (e *CTXTransError) Unwrap() error {
	return e.Cause
}

func (e *CTXTransError) WithContext(key string, value any) *CTXTransError {
	e.Context[key] = value
	return e
}

func (t ErrorType) String() string {
	switch t {
	case ErrFileNotFound:
		return "FileNotFound"
	case ErrFileRead:
		return "FileRead"
	case ErrFileWrite:
		return "FileWrite"
	case ErrParse:
		return "Parse"
	case ErrAPI:
		return "API"
	case ErrValidation:
		return "Validation"
	case ErrConfig:
		return "Config"
	case ErrNetwork:
		return "Network"
	case ErrTranslation:
		return "Translation"
	case ErrUnknown:
		return "Unknown"
	default:
		return "Unknown"
	}
}

type ErrorHandler interface {
	Handle(err error) bool
	GetAdvice(err *CTXTransError) string
}

type DefaultErrorHandler struct{}

func NewDefaultErrorHandler() ErrorHandler {
	return &DefaultErrorHandler{}
}

func (h *DefaultErrorHandler) Handle(err error) bool {
	ctxErr, ok := err.(*CTXTransError)
	if !ok {
		log.Error("Unknown Error: %v", err)
		return false
	}

	advice := h.GetAdvice(ctxErr)
	log.Error("Error Detail: %v\n advice: %s", err, advice)

	return true
}

// GetAdvice returns error handling advice
func (h *DefaultErrorHandler) GetAdvice(err *CTXTransError) string {
	switch err.Type {
	case ErrFileNotFound:
		return "Please check that the file path is correct and ensure the file exists with read permissions"
	case ErrFileRead:
		return "Please check file permissions to ensure read access and verify the file is not corrupted"
	case ErrFileWrite:
		return "Please ensure the output directory exists and has write permissions"
	case ErrParse:
		return "Please verify the file format is correct—NFO files should be XML format and subtitle files should be SRT"
	case ErrAPI:
		return "Please check if the API key is correct, network connectivity is normal, or review the API service status"
	case ErrNetwork:
		return "Please check network connectivity to ensure access to the API service"
	case ErrValidation:
		return "Please verify input parameters are correct—file paths cannot be empty"
	case ErrConfig:
		return "Please check that configuration files or environment variables are set correctly"
	case ErrTranslation:
		return "An issue occurred during translation—possibly due to overly long text or API limits; try reducing batch size"
	default:
		return "Please review detailed error information and check relevant configuration and files"
	}
}

func IsErrorType(err error, errorType ErrorType) bool {
	var ctxErr *CTXTransError
	if errors.As(err, &ctxErr) {
		return ctxErr.Type == errorType
	}
	return false
}

func WrapError(err error, errorType ErrorType, message string) *CTXTransError {
	return NewErrorWithCause(errorType, message, err)
}

func Must(err error, message string) {
	if err != nil {
		panic(fmt.Sprintf("%s: %v", message, err))
	}
}

func SafeExecute(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = NewError(ErrUnknown, fmt.Sprintf("runtime error: %v", r))
		}
	}()

	return fn()
}
