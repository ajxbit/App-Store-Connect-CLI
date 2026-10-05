package errfmt

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/readonly"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/urlsanitize"
)

type ClassifiedError struct {
	Message string
	Hint    string
}

const (
	requestTimeoutHint = "Increase the request timeout (e.g. set `ASC_TIMEOUT=90s`)."
	uploadTimeoutHint  = "Increase the upload timeout (e.g. set `ASC_UPLOAD_TIMEOUT=600s`)."
	systemStatusHint   = "Check Apple's service health with `asc system-status --service \"App Store Connect\"`."
	networkHint        = "Check your network connection and proxy settings (HTTPS_PROXY), then retry."
)

func Classify(err error) ClassifiedError {
	if err == nil {
		return ClassifiedError{}
	}

	// A read-only refusal is a policy decision, not a command failure: render
	// the refusal itself so the line stays stable regardless of which command
	// wrapped it, and add no hint because the message already names the cause.
	if refused, ok := errors.AsType[*readonly.RefusedError](err); ok {
		return ClassifiedError{Message: refused.Error()}
	}

	// A missing Apple web session carries its own next step; the App Store
	// Connect API credential hint below would send the caller to the wrong
	// sign-in.
	if missing, ok := errors.AsType[*shared.MissingWebSessionError](err); ok {
		return ClassifiedError{Message: err.Error(), Hint: missing.Hint}
	}

	if errors.Is(err, shared.ErrMissingAuth) {
		return ClassifiedError{
			Message: err.Error(),
			Hint:    "Run `asc auth login` or `asc auth init` (or set ASC_KEY_ID/ASC_ISSUER_ID/ASC_PRIVATE_KEY_PATH). Try `asc auth doctor` if you're unsure what's misconfigured.",
		}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		// A wait that already names its --timeout flag needs no second,
		// conflicting knob.
		if strings.Contains(err.Error(), "--timeout") {
			return ClassifiedError{Message: err.Error()}
		}
		hint := requestTimeoutHint
		if isUploadTimeoutError(err) {
			hint = uploadTimeoutHint
		}
		return ClassifiedError{
			Message: err.Error(),
			Hint:    hint,
		}
	}

	if IsNetworkFailure(err) {
		return ClassifiedError{Message: err.Error(), Hint: networkHint}
	}

	var apiErr *asc.APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatusCode() >= 500 {
		return ClassifiedError{
			Message: err.Error(),
			Hint:    systemStatusHint,
		}
	}

	if containsPrivacyError(err) {
		return ClassifiedError{
			Message: err.Error(),
			Hint:    "App privacy declarations (data usages) are not available via the public API. Use `asc web privacy pull|plan|apply|publish` or complete App Privacy in the App Store Connect web UI: https://appstoreconnect.apple.com",
		}
	}

	// API-level remediation is already part of the rendered error. Do not add
	// the generic permission hint as well: agreement-blocked 403s are account
	// state, not an API-key role problem, and the two messages conflict.
	var remediationErr *asc.APIError
	if errors.As(err, &remediationErr) && strings.TrimSpace(remediationErr.Remediation) != "" {
		return ClassifiedError{Message: err.Error()}
	}

	if errors.Is(err, asc.ErrForbidden) {
		return ClassifiedError{
			Message: err.Error(),
			Hint:    "Check that your API key has the right role/permissions for this operation in App Store Connect.",
		}
	}

	if errors.Is(err, asc.ErrUnauthorized) {
		return ClassifiedError{
			Message: err.Error(),
			Hint:    "Your credentials may be invalid or expired. Try `asc auth status` and re-login if needed.",
		}
	}

	return ClassifiedError{
		Message: err.Error(),
		Hint:    "",
	}
}

// IsNetworkFailure reports whether err is a failure to reach a server (dial,
// DNS, TLS, proxy, or a dropped connection) rather than a server response or
// local work. It matches the transport error types, never a bare io.EOF, so a
// truncated local file is not mistaken for a network problem.
func IsNetworkFailure(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	_, isURLError := errors.AsType[*url.Error](err)
	_, isTransportError := errors.AsType[*urlsanitize.TransportError](err)
	_, isOpError := errors.AsType[*net.OpError](err)
	return isURLError || isTransportError || isOpError
}

func isUploadTimeoutError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "upload failed") ||
		strings.Contains(msg, "upload operation") ||
		strings.Contains(msg, "multipart upload") ||
		strings.Contains(msg, "s3 upload")
}

// containsPrivacyError checks whether the error references app data usage /
// privacy declaration resources that are not manageable via the API.
func containsPrivacyError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "appdatausages") || strings.Contains(msg, "appdatausagespublications")
}

func FormatStderr(err error) string {
	ce := Classify(err)
	if ce.Message == "" {
		return ""
	}
	if ce.Hint == "" {
		return fmt.Sprintf("Error: %s\n", ce.Message)
	}
	return fmt.Sprintf("Error: %s\nHint: %s\n", ce.Message, ce.Hint)
}
