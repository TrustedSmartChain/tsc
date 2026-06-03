package types

import "fmt"

// DefaultIndex is the default global index
const DefaultIndex uint64 = 1

// DefaultGenesis returns the default genesis state
func DefaultGenesis() *GenesisState {
	return &GenesisState{

		Params: DefaultParams(),
	}
}

// Validate performs basic genesis state validation returning an error upon any
// failure.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	// A live epoch distribution must carry a non-empty root.
	for _, ed := range gs.EpochDistributions {
		if ed.Status == DISTRIBUTION_STATUS_LIVE && len(ed.MerkleRoot) == 0 {
			return fmt.Errorf("live epoch distribution %d has empty merkle root", ed.Epoch)
		}
	}

	// Votes must carry a non-empty root.
	for _, v := range gs.Votes {
		if len(v.MerkleRoot) == 0 {
			return fmt.Errorf("vote for epoch %d signer %s has empty merkle root", v.Epoch, v.Signer)
		}
	}

	return nil
}
