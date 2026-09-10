package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/spf13/cast"

	"github.com/cosmos/cosmos-sdk/client/flags"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"

	evmserverconfig "github.com/cosmos/evm/server/config"
	srvflags "github.com/cosmos/evm/server/flags"
)

// eip155ChainID matches a cosmos chain id of the form "<name>_<eip155>-<revision>"
// (for example "tsc_8878788-1") and captures the EIP-155 number.
var eip155ChainID = regexp.MustCompile(`^[a-z]+_([1-9][0-9]*)-[1-9][0-9]*$`)

// EVMChainIDFromChainID derives the EVM (EIP-155) chain id from a cosmos chain
// id that follows the "<name>_<eip155>-<revision>" convention. ok is false when
// the chain id does not follow it.
func EVMChainIDFromChainID(chainID string) (id uint64, ok bool) {
	m := eip155ChainID.FindStringSubmatch(chainID)
	if m == nil {
		return 0, false
	}
	id, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// ChainIDFromOpts resolves the cosmos chain id. The genesis file under --home
// is authoritative: it is what CometBFT stamps into every block, so a node
// cannot legitimately run under any other id. A --chain-id flag is accepted
// only when it agrees with genesis; a contradicting flag is an error rather
// than an override. The flag alone is honoured only when no genesis file can
// be read, which is the case for the test helpers.
func ChainIDFromOpts(appOpts servertypes.AppOptions) (string, error) {
	flagChainID := cast.ToString(appOpts.Get(flags.FlagChainID))

	genesisChainID, err := chainIDFromGenesis(appOpts)
	switch {
	case err == nil && flagChainID != "" && flagChainID != genesisChainID:
		return "", fmt.Errorf(
			"--%s %q contradicts chain id %q in the genesis file; genesis is authoritative, drop or fix the flag",
			flags.FlagChainID, flagChainID, genesisChainID,
		)
	case err == nil:
		return genesisChainID, nil
	case flagChainID != "":
		return flagChainID, nil
	default:
		return "", err
	}
}

// chainIDFromGenesis reads the chain_id field of the genesis file under
// --home, honouring the genesis_file override the same way the SDK does.
func chainIDFromGenesis(appOpts servertypes.AppOptions) (string, error) {
	genesisPath := cast.ToString(appOpts.Get("genesis_file"))
	if genesisPath == "" {
		genesisPath = filepath.Join("config", "genesis.json")
	}
	if !filepath.IsAbs(genesisPath) {
		genesisPath = filepath.Join(cast.ToString(appOpts.Get(flags.FlagHome)), genesisPath)
	}

	f, err := os.Open(genesisPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	return genutiltypes.ParseChainIDFromGenesis(f)
}

// EVMChainIDFromOpts derives the EVM chain id from the cosmos chain id
// resolved by ChainIDFromOpts. ok is false when no chain id can be resolved or
// it carries no EIP-155 suffix.
func EVMChainIDFromOpts(appOpts servertypes.AppOptions) (uint64, bool) {
	chainID, err := ChainIDFromOpts(appOpts)
	if err != nil {
		return 0, false
	}
	return EVMChainIDFromChainID(chainID)
}

// ResolveEVMChainID picks the EVM chain id the app is built with. The EVM
// chain id is consensus critical (it is part of every EVM signature), so the
// chain id from genesis (see ChainIDFromOpts) is authoritative: when it
// carries an EIP-155 suffix that number is used and a contradicting app.toml /
// --evm.evm-chain-id value is an error. Without a suffix the configured value is used, and with nothing
// configured at all (the throwaway app built for the encoding config has no
// genesis) the cosmos/evm default applies, which is the one value
// SetChainConfig allows the real app to overwrite later in the process.
func ResolveEVMChainID(appOpts servertypes.AppOptions) (uint64, error) {
	configured := cast.ToUint64(appOpts.Get(srvflags.EVMChainID))

	chainID, err := ChainIDFromOpts(appOpts)
	switch {
	case err == nil:
		if derived, ok := EVMChainIDFromChainID(chainID); ok {
			if configured != 0 && configured != evmserverconfig.DefaultEVMChainID && configured != derived {
				return 0, fmt.Errorf(
					"evm chain id %d from app.toml or --%s contradicts %d derived from chain id %q; "+
						"the EIP-155 suffix of the chain id is authoritative, fix or remove the configured value",
					configured, srvflags.EVMChainID, derived, chainID,
				)
			}
			return derived, nil
		}
		// A chain id without an EIP-155 suffix: fall through to the
		// configured value.
	case errors.Is(err, os.ErrNotExist):
		// No genesis file and no --chain-id: the throwaway app built for the
		// encoding config, or a bare test app. Fall through.
	default:
		// A contradicting --chain-id or an unreadable genesis must never be
		// papered over with a fallback value.
		return 0, err
	}

	if configured != 0 {
		return configured, nil
	}
	return evmserverconfig.DefaultEVMChainID, nil
}

// mustResolveEVMChainID is ResolveEVMChainID for the app constructor, which
// cannot return an error.
func mustResolveEVMChainID(appOpts servertypes.AppOptions) uint64 {
	id, err := ResolveEVMChainID(appOpts)
	if err != nil {
		panic(err)
	}
	return id
}
