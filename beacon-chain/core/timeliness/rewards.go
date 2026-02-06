package timeliness

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	log "github.com/sirupsen/logrus"
)

// ProcessTimelinessRewards processes timeliness-based rewards at epoch boundaries.
// It processes rewards only for the previous epoch's blocks, whose attestation inclusion
// window is now closed, ensuring all votes have been collected.
func ProcessTimelinessRewards(ctx context.Context, beaconState state.BeaconState) error {
	cfg := params.BeaconConfig()
	if !cfg.TimelinessRewardEnabled {
		return nil
	}

	tracker := GlobalTracker()
	// Only process rewards for the previous epoch — those blocks' inclusion windows are closed.
	prevVotes := tracker.GetPreviousEpochVotes()

	if len(prevVotes) == 0 {
		return nil
	}

	// Process rewards for each block that received votes
	for blockRoot, votes := range prevVotes {
		if votes.TotalVotes == 0 {
			continue
		}

		// Calculate the m-th smallest timeliness
		mthTimeliness := tracker.CalculateMthSmallestTimeliness(votes)

		// Calculate the reward based on timeliness
		reward := CalculateReward(mthTimeliness)
		if reward == 0 {
			continue
		}

		// Increase the proposer's balance
		if err := helpers.IncreaseBalance(beaconState, votes.ProposerIndex, reward); err != nil {
			log.WithError(err).WithFields(log.Fields{
				"proposerIndex": votes.ProposerIndex,
				"blockRoot":     blockRoot,
				"reward":        reward,
			}).Error("Failed to increase proposer balance for timeliness reward")
			continue
		}

		log.WithFields(log.Fields{
			"proposerIndex":  votes.ProposerIndex,
			"slot":           votes.Slot,
			"epoch":          slots.ToEpoch(votes.Slot),
			"mthTimeliness":  mthTimeliness.String(),
			"reward":         reward,
			"totalVotes":     votes.TotalVotes,
			"expectedVoters": votes.ExpectedVoters,
			"voteDistribution": log.Fields{
				"0-1s": votes.VoteCounts[0],
				"1-2s": votes.VoteCounts[1],
				"2-3s": votes.VoteCounts[2],
				"3-4s": votes.VoteCounts[3],
				"4s+":  votes.VoteCounts[4],
			},
		}).Debug("Processed timeliness reward for proposer")
	}

	return nil
}

// RotateTrackerEpoch rotates the timeliness tracker at epoch boundaries.
// This should be called AFTER ProcessTimelinessRewards. It moves current epoch
// votes to previous (for continued collection) and starts a fresh current map.
func RotateTrackerEpoch(newEpoch primitives.Epoch) {
	GlobalTracker().RotateEpoch(newEpoch)
}
