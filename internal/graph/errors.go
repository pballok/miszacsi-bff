package graph

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const (
	codeBadUserInput   = "BAD_USER_INPUT"
	codeNotFound       = "NOT_FOUND"
	codeTooManyResults = "TOO_MANY_RESULTS"
	codeInternalError  = "INTERNAL_SERVER_ERROR"

	internalErrorMessage = "internal server error"
)

// clientError is an expected, client-actionable query error with a stable extensions.code.
type clientError struct {
	code  string
	field string
	msg   string
}

func (e *clientError) Error() string { return e.msg }

// badUserInput reports an invalid query argument.
func badUserInput(field, msg string) error {
	return &clientError{code: codeBadUserInput, field: field, msg: msg}
}

// notFound reports that the requested entity does not exist.
func notFound(msg string) error {
	return &clientError{code: codeNotFound, msg: msg}
}

// tooManyResults reports that a list query matched more results than it may return.
func tooManyResults(msg string) error {
	return &clientError{code: codeTooManyResults, msg: msg}
}

// internalError marks an unexpected resolver error so the presenter masks it.
type internalError struct {
	err    error
	logged bool // set for recovered panics, which recoverFunc already logged
}

func (e *internalError) Error() string { return e.err.Error() }
func (e *internalError) Unwrap() error { return e.err }

// markInternalErrors is a field middleware tagging every resolver error that is not a
// clientError. gqlgen wraps resolver errors and its own argument coercion errors the same way,
// so the tag is what lets the presenter mask only the former.
func markInternalErrors(ctx context.Context, next graphql.Resolver) (any, error) {
	res, err := next(ctx)
	if err == nil {
		return res, nil
	}
	if _, ok := errors.AsType[*clientError](err); ok {
		return res, err
	}
	if _, ok := errors.AsType[*internalError](err); ok {
		return res, err
	}
	return res, &internalError{err: err}
}

// errorPresenter keeps client errors and gqlgen's own parse, validation and coercion errors,
// and replaces anything else with a generic internal error after logging it.
func errorPresenter(logger *slog.Logger) graphql.ErrorPresenterFunc {
	return func(ctx context.Context, err error) *gqlerror.Error {
		if ce, ok := errors.AsType[*clientError](err); ok {
			gqlErr := located(ctx, err, ce.msg, ce.code)
			if ce.field != "" {
				gqlErr.Extensions["field"] = ce.field
			}
			return gqlErr
		}
		ie, internal := errors.AsType[*internalError](err)
		if _, isGQL := errors.AsType[*gqlerror.Error](err); isGQL && !internal {
			return graphql.DefaultErrorPresenter(ctx, err)
		}

		if !internal || !ie.logged {
			logger.ErrorContext(ctx, "graphql resolver error", "err", err)
		}
		return located(ctx, err, internalErrorMessage, codeInternalError)
	}
}

// located returns a new error with msg and code, keeping only the path and locations of err.
// The original error is left untouched so its details never reach the client.
func located(ctx context.Context, err error, msg, code string) *gqlerror.Error {
	src := graphql.DefaultErrorPresenter(ctx, err)
	return &gqlerror.Error{
		Message:    msg,
		Path:       src.Path,
		Locations:  src.Locations,
		Extensions: map[string]any{"code": code},
	}
}

// recoverFunc logs a resolver panic with its stack and turns it into a masked internal error.
func recoverFunc(logger *slog.Logger) graphql.RecoverFunc {
	return func(ctx context.Context, p any) error {
		logger.ErrorContext(ctx, "graphql resolver panic", "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
		return &internalError{err: fmt.Errorf("panic: %v", p), logged: true}
	}
}
