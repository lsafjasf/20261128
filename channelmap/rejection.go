package channelmap

import (
	"errors"
	"fmt"
	"strings"
)

// RejectionCode identifies why a mapping is unsafe to deliver.
type RejectionCode string

const (
	RejectOverlap    RejectionCode = "overlap"
	RejectIncomplete RejectionCode = "incomplete"
	RejectMismatch   RejectionCode = "mismatch"
	RejectInvalid    RejectionCode = "invalid"
)

// Issue is one actionable validation finding.
type Issue struct {
	Code    RejectionCode `json:"code"`
	Message string        `json:"message"`
}

// RejectionError is returned when a channel map is not a safe bijection.
type RejectionError struct {
	Issues []Issue `json:"issues"`
}

func (e *RejectionError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "channel map rejected"
	}
	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		parts = append(parts, fmt.Sprintf("%s: %s", issue.Code, issue.Message))
	}
	return "channel map rejected: " + strings.Join(parts, "; ")
}

func rejection(issues ...Issue) error {
	return &RejectionError{Issues: issues}
}

func issue(code RejectionCode, format string, args ...any) Issue {
	return Issue{Code: code, Message: fmt.Sprintf(format, args...)}
}

// IsRejectionCode reports whether err is a RejectionError containing code.
func IsRejectionCode(err error, code RejectionCode) bool {
	var rejected *RejectionError
	if !errors.As(err, &rejected) {
		return false
	}
	for _, issue := range rejected.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
