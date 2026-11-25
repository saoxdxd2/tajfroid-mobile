package lib

import (
	"fmt"
	"strconv"
	"strings"
)

// HLC represents a Hybrid Logical Clock formatted as "<wall_ms>:<logical>:<device_id>".
// Wall milliseconds are zero-padded to 16 digits and logical counter to 6 digits
// so lexicographical ordering matches chronological + causal ordering.
type HLC struct {
	WallMS   int64
	Logical  int64
	DeviceID string
}

func FormatHLC(wallMS, logical int64, deviceID string) string {
	if wallMS < 0 {
		wallMS = 0
	}
	if logical < 0 {
		logical = 0
	}
	return fmt.Sprintf("%016d:%06d:%s", wallMS, logical, strings.TrimSpace(deviceID))
}

func ParseHLC(raw string) (HLC, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), ":", 3)
	if len(parts) != 3 {
		return HLC{}, fmt.Errorf("invalid HLC %q", raw)
	}
	wall, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || wall < 0 {
		return HLC{}, fmt.Errorf("invalid HLC wall %q", parts[0])
	}
	logical, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || logical < 0 {
		return HLC{}, fmt.Errorf("invalid HLC logical %q", parts[1])
	}
	if strings.TrimSpace(parts[2]) == "" {
		return HLC{}, fmt.Errorf("empty HLC device_id")
	}
	return HLC{WallMS: wall, Logical: logical, DeviceID: parts[2]}, nil
}

// CompareHLC returns -1 if a < b, 0 if a == b, +1 if a > b.
func CompareHLC(a, b HLC) int {
	if a.WallMS != b.WallMS {
		if a.WallMS < b.WallMS {
			return -1
		}
		return 1
	}
	if a.Logical != b.Logical {
		if a.Logical < b.Logical {
			return -1
		}
		return 1
	}
	if a.DeviceID != b.DeviceID {
		if a.DeviceID < b.DeviceID {
			return -1
		}
		return 1
	}
	return 0
}

// NextHLC advances a local HLC given physical wallMS and optional remote HLC.
func NextHLC(nowMS int64, prevRaw, remoteRaw, deviceID string) string {
	var prev, remote HLC
	if p, err := ParseHLC(prevRaw); err == nil {
		prev = p
	}
	if r, err := ParseHLC(remoteRaw); err == nil {
		remote = r
	}
	maxWall := nowMS
	if prev.WallMS > maxWall {
		maxWall = prev.WallMS
	}
	if remote.WallMS > maxWall {
		maxWall = remote.WallMS
	}

	var logical int64
	switch {
	case maxWall == prev.WallMS && maxWall == remote.WallMS:
		if prev.Logical >= remote.Logical {
			logical = prev.Logical + 1
		} else {
			logical = remote.Logical + 1
		}
	case maxWall == prev.WallMS:
		logical = prev.Logical + 1
	case maxWall == remote.WallMS:
		logical = remote.Logical + 1
	default:
		logical = 0
	}
	return FormatHLC(maxWall, logical, deviceID)
}

// FieldConflictDecision determines how an incoming field_set operation merges
// against the server's current field HLC.
//
// Rules:
// - If no current server clock exists -> apply ("applied").
// - If incoming.BaseHLC matches current -> causal successor -> apply ("applied").
// - If incoming.HLC <= current -> stale duplicate/older edit -> skip ("stale").
// - If incoming.BaseHLC != current and incoming.HLC > current -> concurrent edit
//   on the same field from two offline devices -> return conflict ("conflict")
//   unless ForceWin is true.
func FieldConflictDecision(currentHLC, baseHLC, incomingHLC string, forceWin bool) (string, error) {
	inc, err := ParseHLC(incomingHLC)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(currentHLC) == "" {
		return "applied", nil
	}
	cur, err := ParseHLC(currentHLC)
	if err != nil {
		return "applied", nil
	}
	if CompareHLC(inc, cur) <= 0 {
		return "stale", nil
	}
	if strings.TrimSpace(baseHLC) == strings.TrimSpace(currentHLC) || forceWin {
		return "applied", nil
	}
	return "conflict", nil
}
