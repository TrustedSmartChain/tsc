package main

import (
	"os"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/spf13/cobra"

	"cosmossdk.io/log"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/config"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdktestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	txmodule "github.com/cosmos/cosmos-sdk/x/auth/tx/config"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"github.com/TrustedSmartChain/tsc/v4/app"
	"github.com/cosmos/evm/crypto/hd"
	evmserverconfig "github.com/cosmos/evm/server/config"
	srvflags "github.com/cosmos/evm/server/flags"
)

// NewRootCmd creates a new root command for chain app. It is called once in the
// main function.
func NewRootCmd() *cobra.Command {
	// we "pre"-instantiate the application for getting the injected/configured encoding configuration
	// note, this is not necessary when using app wiring, as depinject can be directly used (see root_v2.go)
	tempApp := app.NewChainApp(
		log.NewNopLogger(), dbm.NewMemDB(), nil, false, simtestutil.NewAppOptionsWithFlagHome(tempDir()),
	)
	encodingConfig := sdktestutil.TestEncodingConfig{
		InterfaceRegistry: tempApp.InterfaceRegistry(),
		Codec:             tempApp.AppCodec(),
		TxConfig:          tempApp.TxConfig(),
		Amino:             tempApp.LegacyAmino(),
	}

	initClientCtx := client.Context{}.
		WithCodec(encodingConfig.Codec).
		WithInterfaceRegistry(encodingConfig.InterfaceRegistry).
		WithTxConfig(encodingConfig.TxConfig).
		WithLegacyAmino(encodingConfig.Amino).
		WithInput(os.Stdin).
		WithAccountRetriever(authtypes.AccountRetriever{}).
		WithHomeDir(app.DefaultNodeHome).
		WithBroadcastMode(flags.FlagBroadcastMode).
		// Cosmos EVM specific setup
		WithKeyringOptions(hd.EthSecp256k1Option()).
		WithLedgerHasProtobuf(true).
		WithViper("")

	rootCmd := &cobra.Command{
		Use:           version.AppName,
		Short:         version.AppName + " Daemon (server)",
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// set the default command outputs
			cmd.SetOut(cmd.OutOrStdout())
			cmd.SetErr(cmd.ErrOrStderr())

			initClientCtx = initClientCtx.WithCmdContext(cmd.Context())
			initClientCtx, err := client.ReadPersistentCommandFlags(initClientCtx, cmd.Flags())
			if err != nil {
				return err
			}

			initClientCtx, err = config.ReadFromClientConfig(initClientCtx)
			if err != nil {
				return err
			}

			// This needs to go after ReadFromClientConfig, as that function
			// sets the RPC client needed for SIGN_MODE_TEXTUAL. This sign mode
			// is only available if the client is online.
			if !initClientCtx.Offline {
				enabledSignModes := append(tx.DefaultSignModes, signing.SignMode_SIGN_MODE_TEXTUAL)
				txConfigOpts := tx.ConfigOptions{
					EnabledSignModes:           enabledSignModes,
					TextualCoinMetadataQueryFn: txmodule.NewGRPCCoinMetadataQueryFn(initClientCtx),
				}
				txConfig, err := tx.NewTxConfigWithOptions(
					initClientCtx.Codec,
					txConfigOpts,
				)
				if err != nil {
					return err
				}

				initClientCtx = initClientCtx.WithTxConfig(app.NewLegacyAwareTxConfig(txConfig))
			}

			if err := client.SetCmdClientContextHandler(initClientCtx, cmd); err != nil {
				return err
			}

			// The EVM chain id follows the cosmos chain id ("tsc_8878788-1" ->
			// 8878788). Seed a fresh app.toml with it when `init` is given
			// --chain-id, so the file documents the value the node will run
			// with; a chain id without an EIP-155 suffix keeps the cosmos/evm
			// default.
			defaultEVMChainID := uint64(evmserverconfig.DefaultEVMChainID)
			if chainID, _ := cmd.Flags().GetString(flags.FlagChainID); chainID != "" {
				if id, ok := app.EVMChainIDFromChainID(chainID); ok {
					defaultEVMChainID = id
				}
			}

			customAppTemplate, customAppConfig := initAppConfig(defaultEVMChainID)
			customCMTConfig := initCometBFTConfig()

			if err := server.InterceptConfigsPreRunHandler(cmd, customAppTemplate, customAppConfig, customCMTConfig); err != nil {
				return err
			}

			// For commands that take --evm.evm-chain-id (start), resolve the
			// EVM chain id from the genesis chain id once app.toml and the
			// flags are loaded and pin it in viper. The app resolves it on its
			// own too (app.ResolveEVMChainID), but the JSON-RPC server reads
			// evm.evm-chain-id from this viper before the app exists, so
			// without this an app.toml from an older build would make
			// net_version disagree with the state machine. A configured value
			// or a --chain-id that contradicts genesis refuses to start here.
			if cmd.Flags().Lookup(srvflags.EVMChainID) != nil {
				serverCtx := server.GetServerContextFromCmd(cmd)
				evmChainID, err := app.ResolveEVMChainID(serverCtx.Viper)
				if err != nil {
					return err
				}
				serverCtx.Viper.Set(srvflags.EVMChainID, evmChainID)
			}
			return nil
		},
	}

	initRootCmd(rootCmd, tempApp)

	// add keyring to autocli opts
	autoCliOpts := tempApp.AutoCliOpts()
	initClientCtx, _ = config.ReadFromClientConfig(initClientCtx)
	autoCliOpts.ClientCtx = initClientCtx

	if err := autoCliOpts.EnhanceRootCommand(rootCmd); err != nil {
		panic(err)
	}

	return rootCmd
}
