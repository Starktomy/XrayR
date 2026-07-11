// Package common contains common utilities that are shared among other packages.
package common

import "github.com/xtls/xray-core/common/session"

// DisableSpliceCopyForbid marks the inbound session so xray-core will not
// take the kernel splice-copy fast path. Splice copy bypasses the userland
// writers that record traffic stats, so Vision/REALITY flows would silently
// inflate usage reports without this guard.
//
// DisableSpliceCopyForbid marks sess so xray-core will not
// take the kernel splice-copy fast path. Splice copy bypasses
// the userland writers that record traffic stats, so
// Vision/REALITY flows would silently inflate usage reports
// without this guard.
//
// Safe to call on a nil session (no-op), since callers
// usually obtain the inbound via session.InboundFromContext
// which can return nil.
func DisableSpliceCopyForbid(sess *session.Inbound) {
	if sess == nil {
		return
	}
	// 3 = cannot splice (see xray-core common/session/session.go).
	sess.CanSpliceCopy = 3
}
