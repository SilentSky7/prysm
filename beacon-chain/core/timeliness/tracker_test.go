package timeliness

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
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
	tracker.ResetForEpoch(1)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

	blockRoot := [32]byte{1, 2, 3}
	proposerIndex := primitives.ValidatorIndex(100)
	// Slot in epoch 1
	slot := primitives.Slot(uint64(slotsPerEpoch) + 10)

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

func TestTracker_RecordVote_RoutesToCorrectMap(t *testing.T) {
	tracker := NewTracker()
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	tracker.ResetForEpoch(2) // currentEpoch = 2, previousEpoch = 1

	currentRoot := [32]byte{1}
	previousRoot := [32]byte{2}
	oldRoot := [32]byte{3}

	// Block in current epoch (epoch 2) → currentVotes
	currentSlot := primitives.Slot(uint64(slotsPerEpoch) * 2)
	tracker.RecordVote(currentRoot, 100, currentSlot, primitives.TimelinessInterval0to1, 5, 100)

	// Block in previous epoch (epoch 1) → previousVotes
	previousSlot := primitives.Slot(uint64(slotsPerEpoch))
	tracker.RecordVote(previousRoot, 101, previousSlot, primitives.TimelinessInterval1to2, 3, 100)

	// Block in epoch 0 (too old) → discarded
	oldSlot := primitives.Slot(5)
	tracker.RecordVote(oldRoot, 102, oldSlot, primitives.TimelinessInterval0to1, 2, 100)

	// Verify current epoch vote
	votes, exists := tracker.GetBlockVotes(currentRoot)
	require.Equal(t, true, exists)
	require.Equal(t, uint64(5), votes.TotalVotes)

	// Verify previous epoch vote
	votes, exists = tracker.GetBlockVotes(previousRoot)
	require.Equal(t, true, exists)
	require.Equal(t, uint64(3), votes.TotalVotes)

	// Verify old vote was discarded
	_, exists = tracker.GetBlockVotes(oldRoot)
	require.Equal(t, false, exists)
}

func TestTracker_RotateEpoch(t *testing.T) {
	tracker := NewTracker()
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	tracker.ResetForEpoch(1)

	epoch1Root := [32]byte{1}
	epoch1Slot := primitives.Slot(uint64(slotsPerEpoch))
	tracker.RecordVote(epoch1Root, 100, epoch1Slot, primitives.TimelinessInterval0to1, 50, 100)

	// At end of epoch 1, rotate to epoch 2.
	// epoch1Root should move from currentVotes to previousVotes.
	tracker.RotateEpoch(2)

	require.Equal(t, primitives.Epoch(2), tracker.CurrentEpoch())

	// epoch1Root should now be in previousVotes and still accessible.
	votes, exists := tracker.GetBlockVotes(epoch1Root)
	require.Equal(t, true, exists)
	require.Equal(t, uint64(50), votes.TotalVotes)

	// GetPreviousEpochVotes should return epoch1Root.
	prevVotes := tracker.GetPreviousEpochVotes()
	require.Equal(t, 1, len(prevVotes))
	require.NotNil(t, prevVotes[epoch1Root])

	// Can still add votes for epoch1Root (it's in previousVotes, epoch 1 = currentEpoch-1).
	tracker.RecordVote(epoch1Root, 100, epoch1Slot, primitives.TimelinessInterval0to1, 20, 100)
	votes, exists = tracker.GetBlockVotes(epoch1Root)
	require.Equal(t, true, exists)
	require.Equal(t, uint64(70), votes.TotalVotes) // 50 + 20

	// Add a new block in epoch 2 (current).
	epoch2Root := [32]byte{2}
	epoch2Slot := primitives.Slot(uint64(slotsPerEpoch) * 2)
	tracker.RecordVote(epoch2Root, 101, epoch2Slot, primitives.TimelinessInterval1to2, 30, 100)

	// Rotate to epoch 3: epoch1Root should be gone, epoch2Root moves to previous.
	tracker.RotateEpoch(3)

	require.Equal(t, primitives.Epoch(3), tracker.CurrentEpoch())

	// epoch1Root should no longer be accessible.
	_, exists = tracker.GetBlockVotes(epoch1Root)
	require.Equal(t, false, exists)

	// epoch2Root should now be in previousVotes.
	prevVotes = tracker.GetPreviousEpochVotes()
	require.Equal(t, 1, len(prevVotes))
	require.NotNil(t, prevVotes[epoch2Root])
	require.Equal(t, uint64(30), prevVotes[epoch2Root].TotalVotes)
}

func TestTracker_CrossEpochAttestationCollection(t *testing.T) {
	// Simulate the full lifecycle:
	// Epoch 0: block proposed, half attestations arrive
	// Epoch 1: remaining attestations arrive, epoch 0 rewards processed
	tracker := NewTracker()
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	tracker.ResetForEpoch(0)

	blockRoot := [32]byte{0xAB}
	blockSlot := primitives.Slot(uint64(slotsPerEpoch) - 1) // last slot of epoch 0
	proposer := primitives.ValidatorIndex(42)

	// During epoch 0: 50 votes arrive
	tracker.RecordVote(blockRoot, proposer, blockSlot, primitives.TimelinessInterval0to1, 50, 100)

	// End of epoch 0: rotate to epoch 1
	// Previous epoch data would be processed, but epoch 0 is in currentVotes and moves to previousVotes
	tracker.RotateEpoch(1)

	// During epoch 1: 30 more votes arrive for the same block (it's now in previousVotes)
	tracker.RecordVote(blockRoot, proposer, blockSlot, primitives.TimelinessInterval0to1, 30, 100)

	// At end of epoch 1: process epoch 0 rewards
	prevVotes := tracker.GetPreviousEpochVotes()
	require.Equal(t, 1, len(prevVotes))
	require.Equal(t, uint64(80), prevVotes[blockRoot].TotalVotes) // 50 + 30

	// m = 100 * 2/3 = 66, we have 80 votes at interval 0 → reward should be max
	mth := tracker.CalculateMthSmallestTimeliness(prevVotes[blockRoot])
	require.Equal(t, primitives.TimelinessInterval0to1, mth)
}

func TestTracker_ResetForEpoch(t *testing.T) {
	tracker := NewTracker()
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	tracker.ResetForEpoch(1)

	blockRoot := [32]byte{1, 2, 3}
	slot := primitives.Slot(uint64(slotsPerEpoch))
	tracker.RecordVote(blockRoot, 100, slot, primitives.TimelinessInterval0to1, 5, 100)

	allVotes := tracker.GetAllBlockVotes()
	require.Equal(t, 1, len(allVotes))

	tracker.ResetForEpoch(5)
	require.Equal(t, 0, len(tracker.GetAllBlockVotes()))
	require.Equal(t, 0, len(tracker.GetPreviousEpochVotes()))
	require.Equal(t, primitives.Epoch(5), tracker.CurrentEpoch())
}

func TestTracker_GetAllBlockVotes(t *testing.T) {
	tracker := NewTracker()
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	tracker.ResetForEpoch(1)

	// Block in current epoch (epoch 1)
	blockRoot1 := [32]byte{1}
	slot1 := primitives.Slot(uint64(slotsPerEpoch))
	tracker.RecordVote(blockRoot1, 100, slot1, primitives.TimelinessInterval0to1, 5, 100)

	// Rotate to epoch 2 → blockRoot1 moves to previousVotes
	tracker.RotateEpoch(2)

	// Block in current epoch (epoch 2)
	blockRoot2 := [32]byte{2}
	slot2 := primitives.Slot(uint64(slotsPerEpoch) * 2)
	tracker.RecordVote(blockRoot2, 101, slot2, primitives.TimelinessInterval1to2, 3, 100)

	// GetAllBlockVotes should return both
	allVotes := tracker.GetAllBlockVotes()
	require.Equal(t, 2, len(allVotes))
	require.NotNil(t, allVotes[blockRoot1])
	require.NotNil(t, allVotes[blockRoot2])
}
