// Package timeliness provides tracking of block timeliness votes for dynamic proposer rewards.
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

// Tracker tracks timeliness votes for blocks across an epoch.
// It aggregates votes from attestations and calculates rewards at epoch boundaries.
type Tracker struct {
	mu sync.RWMutex
	// votes maps block root to its vote data.
	votes map[[32]byte]*BlockVotes
	// currentEpoch tracks which epoch we're collecting votes for.
	currentEpoch primitives.Epoch
}

// NewTracker creates a new timeliness tracker.
func NewTracker() *Tracker {
	return &Tracker{
		votes: make(map[[32]byte]*BlockVotes),
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

	votes, exists := t.votes[blockRoot]
	if !exists {
		votes = &BlockVotes{
			ProposerIndex:  proposerIndex,
			Slot:           slot,
			ExpectedVoters: expectedVoters,
		}
		t.votes[blockRoot] = votes
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

// GetBlockVotes returns the vote data for a specific block.
func (t *Tracker) GetBlockVotes(blockRoot [32]byte) (*BlockVotes, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	votes, exists := t.votes[blockRoot]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid race conditions
	votesCopy := *votes
	return &votesCopy, true
}

// GetAllBlockVotes returns all block votes in the tracker.
func (t *Tracker) GetAllBlockVotes() map[[32]byte]*BlockVotes {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[[32]byte]*BlockVotes, len(t.votes))
	for root, votes := range t.votes {
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
	for i := 0; i < 5; i++ {
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

// ResetForEpoch clears all votes and prepares the tracker for a new epoch.
func (t *Tracker) ResetForEpoch(epoch primitives.Epoch) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.votes = make(map[[32]byte]*BlockVotes)
	t.currentEpoch = epoch
}

// CurrentEpoch returns the current epoch being tracked.
func (t *Tracker) CurrentEpoch() primitives.Epoch {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.currentEpoch
}
