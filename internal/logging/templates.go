package logging

import "fmt"

func MsgYadiskAuthError() string {
	return "yadisk authorization error"
}

func MsgYadiskUpstreamUnstable() string {
	return "yadisk unstable upstream or network"
}

func MsgYadiskRetriesExhausted() string {
	return "yadisk retries exhausted by transient status"
}

func MsgUpdatePanic() string {
	return "panic in update handler"
}

func MsgUserFlowAuthFailed() string {
	return "user flow failed by upstream authorization"
}

func NewRequestID(updateID int) string {
	return fmt.Sprintf("upd-%d", updateID)
}
