package common

import (
	"testing"

	"github.com/xtls/xray-core/common/session"
)

// DisableSpliceCopyForbid is a tiny helper; the tests focus on
// the safety contract (nil-safe) and the value it sets.

func TestDisableSpliceCopyForbidNil(t *testing.T) {
	// Must not panic on a nil inbound.
	DisableSpliceCopyForbid(nil)
}

func TestDisableSpliceCopyForbidSetsValue(t *testing.T) {
	sess := &session.Inbound{}
	DisableSpliceCopyForbid(sess)
	if sess.CanSpliceCopy != 3 {
		t.Errorf("CanSpliceCopy = %d, want 3 (= cannot splice)", sess.CanSpliceCopy)
	}
}
