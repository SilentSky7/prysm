// Package timeliness provides tracking of block timeliness votes for dynamic proposer rewards.
//
// Because attestations for a block may span two epochs (an attestation's inclusion window is
// SLOTS_PER_EPOCH slots), the tracker maintains two vote maps:
//   - currentVotes: votes for blocks proposed in the current epoch
//   - previousVotes: votes for blocks proposed in the previous epoch
//
// At each epoch boundary (end of epoch N):
//  1. Process rewards from previousVotes (epoch N-1 blocks; their inclusion window is now closed).
//  2. Rotate: previousVotes = currentVotes (epoch N blocks still accepting attestations).
//  3. Clear currentVotes for epoch N+1.
package timeliness

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// BlockVotes holds the timeliness vote distribution for a specific block.
type BlockVotes struct {
	// ProposerIndex is the validator index of the block proposer.
	ProposerIndex primitives.ValidatorIndex
	// Slot is the slot in which the block was proposed.
	Slot primitives.Slot
	// VoteCounts tracks the number of votes for each timeliness interval.
	// Index 0: 0-1s, Index 1: 1-2s, Index 2: 2-3s, Index 3: 3-4s, Index 4: 4s+
	VoteCounts [5]uint64
	// TotalVotes is the total number of votes received for this block.
	TotalVotes uint64
	// ExpectedVoters is the number of validators expected to vote on this block.
	// This is used to calculate m = ExpectedVoters * MFraction.
	ExpectedVoters uint64
}

// Tracker tracks timeliness votes for blocks across epochs.
// It maintains two vote windows to handle attestations that span epoch boundaries:
//   - currentVotes: accumulates votes for blocks in the current epoch
//   - previousVotes: accumulates remaining votes for blocks in the previous epoch
//
// A processedBlocks set ensures each containing block's attestations are recorded
// exactly once, preventing double-counting during state replays.
type Tracker struct {
	mu sync.RWMutex
	// currentVotes maps block root to vote data for blocks in the current epoch.
	currentVotes map[[32]byte]*BlockVotes
	// previousVotes maps block root to vote data for blocks in the previous epoch.
	// These blocks' inclusion windows may still be open at the start of the current epoch.
	previousVotes map[[32]byte]*BlockVotes
	// processedBlocks tracks which containing blocks have already had their
	// attestations recorded, to prevent double-counting during state replays.
	processedBlocks map[[32]byte]bool
	// currentEpoch tracks which epoch we're collecting votes for.
	currentEpoch primitives.Epoch
}

// NewTracker creates a new timeliness tracker.
func NewTracker() *Tracker {
	return &Tracker{
		currentVotes:    make(map[[32]byte]*BlockVotes),
		previousVotes:   make(map[[32]byte]*BlockVotes),
		processedBlocks: make(map[[32]byte]bool),
	}
}

// globalTracker is the singleton tracker instance.
var (
	globalTracker     *Tracker
	globalTrackerOnce sync.Once
)

// GlobalTracker returns the global timeliness tracker instance.
func GlobalTracker() *Tracker {
	globalTrackerOnce.Do(func() {
		globalTracker = NewTracker()
	})
	return globalTracker
}

// RecordVote records a timeliness vote for a block.
// The vote is placed into the appropriate map (current or previous) based on the
// block's epoch. Votes for blocks older than the previous epoch are discarded.
//
// blockRoot: the root of the block being voted on
// proposerIndex: the proposer of the block
// slot: the slot of the block
// timeliness: the timeliness value from the attestation (0-4)
// voterCount: the number of validators making this vote (from aggregation bits)
// expectedVoters: the total number of validators expected to vote on this slot
func (t *Tracker) RecordVote(
	blockRoot [32]byte,
	proposerIndex primitives.ValidatorIndex,
	slot primitives.Slot,
	timeliness primitives.BlockTimeliness,
	voterCount uint64,
	expectedVoters uint64,
) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Determine which epoch this block belongs to.
	blockEpoch := primitives.Epoch(uint64(slot) / uint64(params.BeaconConfig().SlotsPerEpoch))

	// Select the appropriate vote map.
	var targetVotes map[[32]byte]*BlockVotes
	switch {
	case blockEpoch == t.currentEpoch:
		targetVotes = t.currentVotes
	case t.currentEpoch > 0 && blockEpoch == t.currentEpoch-1:
		targetVotes = t.previousVotes
	default:
		// Block is too old (more than 1 epoch behind) or from the future; discard.
		return
	}

	votes, exists := targetVotes[blockRoot]
	if !exists {
		votes = &BlockVotes{
			ProposerIndex:  proposerIndex,
			Slot:           slot,
			ExpectedVoters: expectedVoters,
		}
		targetVotes[blockRoot] = votes
	}

	// Ensure timeliness is within valid range
	if timeliness > primitives.MaxTimelinessValue {
		timeliness = primitives.MaxTimelinessValue
	}

	votes.VoteCounts[timeliness] += voterCount
	votes.TotalVotes += voterCount

	// Update expected voters if larger value is provided
	if expectedVoters > votes.ExpectedVoters {
		votes.ExpectedVoters = expectedVoters
	}
}

// GetBlockVotes returns the vote data for a specific block, searching both maps.
func (t *Tracker) GetBlockVotes(blockRoot [32]byte) (*BlockVotes, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if votes, exists := t.currentVotes[blockRoot]; exists {
		votesCopy := *votes
		return &votesCopy, true
	}
	if votes, exists := t.previousVotes[blockRoot]; exists {
		votesCopy := *votes
		return &votesCopy, true
	}
	return nil, false
}

// GetPreviousEpochVotes returns all block votes from the previous epoch.
// These votes are ready to be processed for rewards because their inclusion window is closed.
func (t *Tracker) GetPreviousEpochVotes() map[[32]byte]*BlockVotes {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[[32]byte]*BlockVotes, len(t.previousVotes))
	for root, votes := range t.previousVotes {
		votesCopy := *votes
		result[root] = &votesCopy
	}
	return result
}

// GetAllBlockVotes returns all block votes (both current and previous epochs).
func (t *Tracker) GetAllBlockVotes() map[[32]byte]*BlockVotes {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[[32]byte]*BlockVotes, len(t.currentVotes)+len(t.previousVotes))
	for root, votes := range t.previousVotes {
		votesCopy := *votes
		result[root] = &votesCopy
	}
	for root, votes := range t.currentVotes {
		votesCopy := *votes
		result[root] = &votesCopy
	}
	return result
}

// CalculateMthSmallestTimeliness calculates the m-th smallest timeliness value
// based on the vote distribution and configuration.
// m = expectedVoters * MFractionNumerator / MFractionDenominator
func (t *Tracker) CalculateMthSmallestTimeliness(votes *BlockVotes) primitives.BlockTimeliness {
	cfg := params.BeaconConfig()

	// Calculate m based on expected voters (including non-voters)
	m := votes.ExpectedVoters * cfg.TimelinessMFractionNumerator / cfg.TimelinessMFractionDenominator
	if m == 0 {
		m = 1 // At least 1
	}

	return GetMthSmallestTimeliness(votes.VoteCounts, m)
}

// GetMthSmallestTimeliness finds the m-th smallest timeliness value from vote counts.
// voteCounts[i] represents the number of votes for timeliness interval i.
// If there are fewer than m votes, returns the largest timeliness (late).
func GetMthSmallestTimeliness(voteCounts [5]uint64, m uint64) primitives.BlockTimeliness {
	cumulative := uint64(0)
	for i := range 5 {
		cumulative += voteCounts[i]
		if cumulative >= m {
			return primitives.BlockTimeliness(i)
		}
	}
	// If we don't have enough votes, return late
	return primitives.TimelinessIntervalLate
}

// CalculateReward calculates the reward for a proposer based on the m-th smallest timeliness.
func CalculateReward(timeliness primitives.BlockTimeliness) uint64 {
	cfg := params.BeaconConfig()
	if timeliness > primitives.MaxTimelinessValue {
		return 0
	}
	return cfg.TimelinessRewardByInterval[timeliness]
}

// RotateEpoch performs the epoch boundary rotation:
//  1. The caller should first process rewards from previousVotes (inclusion window closed).
//  2. This method then moves currentVotes → previousVotes and starts a fresh currentVotes.
//
// newEpoch is the epoch about to begin (currentEpoch + 1).
func (t *Tracker) RotateEpoch(newEpoch primitives.Epoch) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// The previous epoch data should already have been processed for rewards
	// by the caller. Now rotate.
	t.previousVotes = t.currentVotes
	t.currentVotes = make(map[[32]byte]*BlockVotes)
	t.processedBlocks = make(map[[32]byte]bool)
	t.currentEpoch = newEpoch
}

// ResetForEpoch clears all votes and prepares the tracker for a specific epoch.
// This is useful for initialization or testing.
func (t *Tracker) ResetForEpoch(epoch primitives.Epoch) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.currentVotes = make(map[[32]byte]*BlockVotes)
	t.previousVotes = make(map[[32]byte]*BlockVotes)
	t.processedBlocks = make(map[[32]byte]bool)
	t.currentEpoch = epoch
}

// CurrentEpoch returns the current epoch being tracked.
func (t *Tracker) CurrentEpoch() primitives.Epoch {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.currentEpoch
}

// IsBlockProcessed returns true if attestations from the given containing block
// have already been recorded. This prevents double-counting during state replays.
func (t *Tracker) IsBlockProcessed(containingBlockRoot [32]byte) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.processedBlocks[containingBlockRoot]
}

// MarkBlockProcessed marks a containing block as having had its attestations recorded.
func (t *Tracker) MarkBlockProcessed(containingBlockRoot [32]byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.processedBlocks[containingBlockRoot] = true
}
