package timeliness

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestGetMthSmallestTimeliness(t *testing.T) {
	tests := []struct {
		name       string
		voteCounts [5]uint64
		m          uint64
		expected   primitives.BlockTimeliness
	}{
		{
			name:       "m=1 with only 0-1s votes",
			voteCounts: [5]uint64{10, 0, 0, 0, 0},
			m:          1,
			expected:   primitives.TimelinessInterval0to1,
		},
		{
			name:       "m=5 with distributed votes",
			voteCounts: [5]uint64{3, 3, 3, 3, 3},
			m:          5,
			expected:   primitives.TimelinessInterval1to2,
		},
		{
			name:       "m=10 with distributed votes",
			voteCounts: [5]uint64{3, 3, 3, 3, 3},
			m:          10,
			expected:   primitives.TimelinessInterval3to4,
		},
		{
			name:       "m=15 exact boundary",
			voteCounts: [5]uint64{3, 3, 3, 3, 3},
			m:          15,
			expected:   primitives.TimelinessIntervalLate,
		},
		{
			name:       "m exceeds total votes",
			voteCounts: [5]uint64{1, 1, 1, 0, 0},
			m:          10,
			expected:   primitives.TimelinessIntervalLate,
		},
		{
			name:       "2/3 of validators voted 0-1s",
			voteCounts: [5]uint64{67, 33, 0, 0, 0},
			m:          67, // 2/3 of 100
			expected:   primitives.TimelinessInterval0to1,
		},
		{
			name:       "2/3 of validators mostly 1-2s",
			voteCounts: [5]uint64{30, 40, 20, 10, 0},
			m:          67,
			expected:   primitives.TimelinessInterval1to2,
		},
		{
			name:       "2/3 of validators include some late",
			voteCounts: [5]uint64{20, 20, 20, 10, 30},
			m:          67,
			expected:   primitives.TimelinessInterval2to3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetMthSmallestTimeliness(tt.voteCounts, tt.m)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestTracker_RecordVote(t *testing.T) {
	tracker := NewTracker()
	blockRoot := [32]byte{1, 2, 3}
	proposerIndex := primitives.ValidatorIndex(100)
	slot := primitives.Slot(10)

	// Record first vote
	tracker.RecordVote(blockRoot, proposerIndex, slot, primitives.TimelinessInterval0to1, 5, 100)

	votes, exists := tracker.GetBlockVotes(blockRoot)
	require.Equal(t, true, exists)
	require.Equal(t, proposerIndex, votes.ProposerIndex)
	require.Equal(t, slot, votes.Slot)
	require.Equal(t, uint64(5), votes.VoteCounts[0])
	require.Equal(t, uint64(5), votes.TotalVotes)
	require.Equal(t, uint64(100), votes.ExpectedVoters)

	// Record more votes for the same block
	tracker.RecordVote(blockRoot, proposerIndex, slot, primitives.TimelinessInterval1to2, 10, 100)
	tracker.RecordVote(blockRoot, proposerIndex, slot, primitives.TimelinessInterval0to1, 3, 100)

	votes, exists = tracker.GetBlockVotes(blockRoot)
	require.Equal(t, true, exists)
	require.Equal(t, uint64(8), votes.VoteCounts[0])  // 5 + 3
	require.Equal(t, uint64(10), votes.VoteCounts[1]) // 10
	require.Equal(t, uint64(18), votes.TotalVotes)    // 5 + 10 + 3
}

func TestTracker_ResetForEpoch(t *testing.T) {
	tracker := NewTracker()
	blockRoot := [32]byte{1, 2, 3}

	tracker.RecordVote(blockRoot, 100, 10, primitives.TimelinessInterval0to1, 5, 100)
	require.Equal(t, 1, len(tracker.GetAllBlockVotes()))

	tracker.ResetForEpoch(5)
	require.Equal(t, 0, len(tracker.GetAllBlockVotes()))
	require.Equal(t, primitives.Epoch(5), tracker.CurrentEpoch())
}

func TestTracker_GetAllBlockVotes(t *testing.T) {
	tracker := NewTracker()
	blockRoot1 := [32]byte{1}
	blockRoot2 := [32]byte{2}

	tracker.RecordVote(blockRoot1, 100, 10, primitives.TimelinessInterval0to1, 5, 100)
	tracker.RecordVote(blockRoot2, 101, 11, primitives.TimelinessInterval1to2, 3, 100)

	allVotes := tracker.GetAllBlockVotes()
	require.Equal(t, 2, len(allVotes))
	require.NotNil(t, allVotes[blockRoot1])
	require.NotNil(t, allVotes[blockRoot2])
}
