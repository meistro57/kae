// Package runcontrol decides when to continue, stop, or branch a KAE run
// based on novelty decay and anomaly intensity.
package runcontrol

// RunController tracks per-cycle novelty and decides when to stop or branch.
type RunController struct {
	noveltyThreshold float64 // new_nodes/total below this → cycle is stagnant
	stagnationWindow int     // consecutive stagnant cycles before stop signal
	branchThreshold  float64 // anomaly score above this → branch recommended
	maxBranches      int     // 0 = unlimited
	stagnantCount    int
	branchCount      int
}

// New creates a RunController with the given parameters.
//
//   - noveltyThreshold: fraction of new nodes required to count as "novel"
//     (e.g. 0.05 = at least 5 % of total nodes must be new this cycle).
//   - stagnationWindow: how many consecutive stagnant cycles before Stop.
//   - branchThreshold: anomaly score (0-1) above which branching is suggested.
//   - maxBranches: cap on auto-branches (0 = no cap).
func New(noveltyThreshold float64, stagnationWindow int, branchThreshold float64, maxBranches int) *RunController {
	return &RunController{
		noveltyThreshold: noveltyThreshold,
		stagnationWindow: stagnationWindow,
		branchThreshold:  branchThreshold,
		maxBranches:      maxBranches,
	}
}

// RecordNovelty computes new_nodes/total_nodes for this cycle and updates the
// internal stagnation counter.  Returns true if the run should continue.
func (r *RunController) RecordNovelty(newNodes, totalNodes int) bool {
	if totalNodes == 0 {
		return true
	}
	ratio := float64(newNodes) / float64(totalNodes)
	if ratio < r.noveltyThreshold {
		r.stagnantCount++
	} else {
		r.stagnantCount = 0
	}
	return r.stagnantCount < r.stagnationWindow
}

// ShouldBranch returns true when the anomaly score exceeds the threshold and
// we have not yet reached maxBranches.
func (r *RunController) ShouldBranch(anomalyScore float64) bool {
	if r.maxBranches > 0 && r.branchCount >= r.maxBranches {
		return false
	}
	return anomalyScore >= r.branchThreshold
}

// RecordBranch increments the branch counter.  Call this when a branch is
// actually spawned.
func (r *RunController) RecordBranch() { r.branchCount++ }

// StagnantCycles returns the current consecutive-stagnation count.
func (r *RunController) StagnantCycles() int { return r.stagnantCount }

// BranchCount returns how many branches have been spawned so far.
func (r *RunController) BranchCount() int { return r.branchCount }
