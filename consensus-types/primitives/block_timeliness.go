package primitives

import (
	fssz "github.com/prysmaticlabs/fastssz"
)

var _ fssz.HashRoot = (BlockTimeliness)(0)
var _ fssz.Marshaler = (*BlockTimeliness)(nil)
var _ fssz.Unmarshaler = (*BlockTimeliness)(nil)

// BlockTimeliness represents the time window in which a block was received.
// Each value corresponds to a specific time interval:
//   - 0: Block received within 0-1 seconds of slot start
//   - 1: Block received within 1-2 seconds of slot start
//   - 2: Block received within 2-3 seconds of slot start
//   - 3: Block received within 3-4 seconds of slot start
//   - 4: Block received after 4+ seconds (late block)
//
// This is used by validators to vote on block arrival timeliness,
// which can later be used to adjust proposer rewards.
type BlockTimeliness uint8

// BlockTimeliness constants representing time windows.
const (
	// TimelinessInterval0to1 represents block received within 0-1 seconds.
	TimelinessInterval0to1 BlockTimeliness = 0
	// TimelinessInterval1to2 represents block received within 1-2 seconds.
	TimelinessInterval1to2 BlockTimeliness = 1
	// TimelinessInterval2to3 represents block received within 2-3 seconds.
	TimelinessInterval2to3 BlockTimeliness = 2
	// TimelinessInterval3to4 represents block received within 3-4 seconds.
	TimelinessInterval3to4 BlockTimeliness = 3
	// TimelinessIntervalLate represents block received after 4+ seconds.
	TimelinessIntervalLate BlockTimeliness = 4
	// MaxTimelinessValue is the maximum valid timeliness value.
	MaxTimelinessValue BlockTimeliness = 4
)

// TimelinessIntervalDuration is the duration of each timeliness interval in milliseconds.
const TimelinessIntervalDuration = 1000 // 1 second in milliseconds

// TimelinessToBits returns the bit index for the given timeliness value.
// This is used for aggregation where each timeliness value corresponds to a bit position.
func (t BlockTimeliness) ToBitIndex() uint64 {
	return uint64(t)
}

// IsValid returns true if the timeliness value is within the valid range.
func (t BlockTimeliness) IsValid() bool {
	return t <= MaxTimelinessValue
}

// HashTreeRoot returns the hash tree root of the block timeliness.
func (t BlockTimeliness) HashTreeRoot() ([32]byte, error) {
	return fssz.HashWithDefaultHasher(t)
}

// HashTreeRootWith hashes the block timeliness with the given hasher.
func (t BlockTimeliness) HashTreeRootWith(hh *fssz.Hasher) error {
	hh.PutUint8(uint8(t))
	return nil
}

// MarshalSSZ marshals the block timeliness into SSZ format.
func (t *BlockTimeliness) MarshalSSZ() ([]byte, error) {
	return []byte{uint8(*t)}, nil
}

// MarshalSSZTo marshals the block timeliness into the provided buffer.
func (t *BlockTimeliness) MarshalSSZTo(buf []byte) ([]byte, error) {
	return append(buf, uint8(*t)), nil
}

// SizeSSZ returns the SSZ encoded size in bytes.
func (t *BlockTimeliness) SizeSSZ() int {
	return 1
}

// UnmarshalSSZ unmarshals the block timeliness from SSZ format.
func (t *BlockTimeliness) UnmarshalSSZ(buf []byte) error {
	if len(buf) < 1 {
		return fssz.ErrSize
	}
	*t = BlockTimeliness(buf[0])
	return nil
}

// CalculateTimeliness calculates the BlockTimeliness value based on the
// time elapsed since slot start (in milliseconds).
func CalculateTimeliness(elapsedMillis int64) BlockTimeliness {
	if elapsedMillis < 0 {
		return TimelinessInterval0to1
	}
	interval := elapsedMillis / TimelinessIntervalDuration
	if interval > int64(MaxTimelinessValue) {
		return TimelinessIntervalLate
	}
	return BlockTimeliness(interval)
}

// String returns a human-readable representation of the timeliness.
func (t BlockTimeliness) String() string {
	switch t {
	case TimelinessInterval0to1:
		return "0-1s"
	case TimelinessInterval1to2:
		return "1-2s"
	case TimelinessInterval2to3:
		return "2-3s"
	case TimelinessInterval3to4:
		return "3-4s"
	case TimelinessIntervalLate:
		return "4s+"
	default:
		return "unknown"
	}
}
