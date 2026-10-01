package gormstore

import "strconv"

// contentID accepts only positive decimal primary keys, never SQL fragments.
func contentID(raw string) (uint64, bool) {
	if raw == "" {
		return 0, false
	}
	for _, c := range raw {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	return id, err == nil && id > 0
}
