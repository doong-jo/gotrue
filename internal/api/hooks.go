package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/supabase/auth/internal/hooks"

	"github.com/supabase/auth/internal/storage"
)

func (a *API) runHook(ctx context.Context, name string, input, output any) ([]byte, error) {
	db := a.db.WithContext(ctx)

	request, err := json.Marshal(input)
	if err != nil {
		panic(err)
	}

	var response []byte
	if err := db.Transaction(func(tx *storage.Connection) error {
		// We rely on Postgres timeouts to ensure the function doesn't overrun
		if terr := tx.RawQuery(fmt.Sprintf("set local statement_timeout TO '%d';", hooks.DefaultTimeout)).Exec(); terr != nil {
			return terr
		}

		if terr := tx.RawQuery(fmt.Sprintf("select %s(?);", name), request).First(&response); terr != nil {
			return terr
		}

		// reset the timeout
		if terr := tx.RawQuery("set local statement_timeout TO default;").Exec(); terr != nil {
			return terr
		}

		return nil
	}); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(response, output); err != nil {
		return response, err
	}

	return response, nil
}

func (a *API) invokeHook(ctx context.Context, input, output any) error {
	config := a.config
	switch input.(type) {
	case *hooks.MFAVerificationAttemptInput:
		hookOutput, ok := output.(*hooks.MFAVerificationAttemptOutput)
		if !ok {
			panic("output should be *hooks.MFAVerificationAttemptOutput")
		}

		if _, err := a.runHook(ctx, config.Hook.MFAVerificationAttempt.HookName, input, output); err != nil {
			return internalServerError("Error invoking MFA verification hook.").WithInternalError(err)
		}

		if hookOutput.IsError() {
			httpCode := hookOutput.HookError.HTTPCode

			if httpCode == 0 {
				httpCode = http.StatusInternalServerError
			}

			httpError := &HTTPError{
				Code:    httpCode,
				Message: hookOutput.HookError.Message,
			}

			return httpError.WithInternalError(&hookOutput.HookError)
		}

		return nil
	case *hooks.PasswordVerificationAttemptInput:
		hookOutput, ok := output.(*hooks.PasswordVerificationAttemptOutput)
		if !ok {
			panic("output should be *hooks.PasswordVerificationAttemptOutput")
		}

		if _, err := a.runHook(ctx, config.Hook.PasswordVerificationAttempt.HookName, input, output); err != nil {
			return internalServerError("Error invoking password verification hook.").WithInternalError(err)
		}

		if hookOutput.IsError() {
			httpCode := hookOutput.HookError.HTTPCode

			if httpCode == 0 {
				httpCode = http.StatusInternalServerError
			}

			httpError := &HTTPError{
				Code:    httpCode,
				Message: hookOutput.HookError.Message,
			}

			return httpError.WithInternalError(&hookOutput.HookError)
		}

		return nil
	case *hooks.CustomAccessTokenInput:
		hookOutput, ok := output.(*hooks.CustomAccessTokenOutput)
		if !ok {
			panic("output should be *hooks.CustomAccessTokenOutput")
		}

		if _, err := a.runHook(ctx, config.Hook.CustomAccessToken.HookName, input, output); err != nil {
			return internalServerError("Error invoking access token hook.").WithInternalError(err)
		}

		if hookOutput.IsError() {
			httpCode := hookOutput.HookError.HTTPCode

			if httpCode == 0 {
				httpCode = http.StatusInternalServerError
			}

			httpError := &HTTPError{
				Code:    httpCode,
				Message: hookOutput.HookError.Message,
			}

			return httpError.WithInternalError(&hookOutput.HookError)
		}
		if err := validateTokenClaims(hookOutput.Claims); err != nil {
			httpCode := hookOutput.HookError.HTTPCode

			if httpCode == 0 {
				httpCode = http.StatusInternalServerError
			}

			httpError := &HTTPError{
				Code:    httpCode,
				Message: err.Error(),
			}

			return httpError
		}
		return nil

	default:
		panic("unknown hook input type")
	}
}
