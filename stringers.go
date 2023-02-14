package rrl

import (
	"fmt"
)

func (ac AllowanceCategory) String() string {
	switch ac {
	case AllowanceAnswer:
		return "AllowanceAnswer"
	case AllowanceReferral:
		return "AllowanceReferral"
	case AllowanceNoData:
		return "AllowanceNoData"
	case AllowanceNXDomain:
		return "AllowanceNXDomain"
	case AllowanceError:
		return "AllowanceError"
	}

	return fmt.Sprintf("Unstringable AllowanceCategory %d", ac)
}

func (act Action) String() string {
	switch act {
	case Send:
		return "Send"
	case Drop:
		return "Drop"
	case Slip:
		return "Slip"
	}

	return fmt.Sprintf("UnStringable Action %d", act)
}

func (ipr IPReason) String() string {
	switch ipr {
	case IPOk:
		return "IPOk"
	case IPNotConfigured:
		return "IPNotConfigured"
	case IPNotReached:
		return "IPNotReached"
	case IPRateLimit:
		return "IPRateLimit"
	case IPCacheFull:
		return "IPCacheFull"
	}

	return fmt.Sprintf("UnStringable IPReason %d", ipr)
}

func (rtr RTReason) String() string {
	switch rtr {
	case RTOk:
		return "RTOk"
	case RTNotConfigured:
		return "RTNotConfigured"
	case RTRateLimit:
		return "RTRateLimit"
	case RTNotReached:
		return "RTNotReached"
	case RTNotUDP:
		return "RTNotUDP"
	case RTCacheFull:
		return "RTCacheFull"
	}

	return fmt.Sprintf("UnStringable RTReason %d", rtr)
}

func (rt *ResponseTuple) String() string {
	return fmt.Sprintf("%d/%d %s sn=%s",
		rt.Class, rt.Type, rt.AllowanceCategory.String(), rt.SalientName)
}
