package accessgates

import "errors"

type Mode string

const (
	All     Mode = "ALL_REQUIRED"
	Any     Mode = "ANY_REQUIRED"
	Minimum Mode = "MINIMUM_REQUIRED"
)

type Membership struct {
	Status   string
	IsMember bool
	Err      error
}

func Evaluate(enabled, bypass bool, mode Mode, minimum int, channels []Membership) (bool, error) {
	if !enabled || bypass {
		return true, nil
	}
	if mode != All && mode != Any && mode != Minimum {
		return false, errors.New("invalid mode")
	}
	if len(channels) == 0 {
		return false, errors.New("active gate requires channels")
	}
	if mode == Minimum && (minimum < 1 || minimum > len(channels)) {
		return false, errors.New("invalid minimum")
	}
	count := 0
	for _, m := range channels {
		if m.Err != nil {
			return false, m.Err
		}
		if m.Status == "member" || m.Status == "administrator" || m.Status == "creator" || (m.Status == "restricted" && m.IsMember) {
			count++
		}
	}
	switch mode {
	case All:
		return count == len(channels), nil
	case Any:
		return count > 0, nil
	case Minimum:
		return count >= minimum, nil
	}
	return false, errors.New("invalid mode")
}
