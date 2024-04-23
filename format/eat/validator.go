package eat

import (
	"github.com/ethereum/go-ethereum/common"

	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/util/ifutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
)

// tokenValidator offers helper functions to streamline token validation. If accumulateErrors is enabled, it accumulates
// validation errors in an error list. Otherwise, it records only the first error and ignores any subsequent validation
// errors.
type tokenValidator struct {
	token            *Token
	errTemplate      errors.TemplateFn
	accumulateErrors bool
	err              error
}

// require ensures that the given value is "not empty" - see isEmpty() for exact semantics. It returns true if that is
// the case. Otherwise it records an error in the validator's error list and returns false.
func (v *tokenValidator) require(field string, val interface{}) bool {
	if !v.accumulateErrors && v.err != nil {
		return !isEmpty(val)
	}
	if isEmpty(val) {
		v.errorReason(field + " missing")
		return false
	}
	return true
}

// refuse ensures that the given value is "empty" - see isEmpty() for exact semantics. It returns true if that is the
// case. Otherwise it returns false and records an error in the validator's error list.
func (v *tokenValidator) refuse(field string, val interface{}) bool {
	if !v.accumulateErrors && v.err != nil {
		return isEmpty(val)
	}
	if !isEmpty(val) {
		v.errorReason(field + " not allowed")
		return false
	}
	return true
}

// reason adds an error with a name-value pair ("reason", reason) and additional optional fields to the validator's
// error list.
func (v *tokenValidator) errorReason(reason string, fields ...interface{}) {
	if !v.accumulateErrors && v.err != nil {
		return
	}
	v.err = errors.Append(v.err, v.errTemplate("reason", reason).With(fields...))
}

// error appends the given error to the validator's error list (if it is not nil).
func (v *tokenValidator) error(err error) {
	if err == nil {
		return
	}
	if !v.accumulateErrors && v.err != nil {
		return
	}
	v.err = errors.Append(v.err, err)
}

func isEmpty(field interface{}) bool {
	switch t := field.(type) {
	case string:
		return t == ""
	case bool:
		return !t
	case []byte:
		return len(t) == 0
	case id.ID:
		return t.IsNil()
	case hash.Hash:
		return t.IsNil()
	case utc.UTC:
		return t.IsZero()
	case common.Address:
		return t == zeroAddr
	case common.Hash:
		return t == zeroHash
	case map[string]interface{}:
		return len(t) == 0
	case ClientConfirmation:
		return t == zeroCnf
	default:
		return ifutil.IsEmpty(field)
	}
}
