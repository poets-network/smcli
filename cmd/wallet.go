package cmd

import (
	"context"
	"crypto"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/btcsuite/btcutil/base58"
	"github.com/hashicorp/go-secure-stdlib/password"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/oasisprotocol/curve25519-voi/primitives/ed25519"
	pb "github.com/spacemeshos/api/release/go/spacemesh/v1"
	"github.com/spacemeshos/go-spacemesh/common/types"
	"github.com/spacemeshos/go-spacemesh/genvm/sdk"
	walletSdk "github.com/spacemeshos/go-spacemesh/genvm/sdk/wallet"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/spacemeshos/smcli/common"
	"github.com/spacemeshos/smcli/wallet"
)

var (
	// debug indicates that the program is in debug mode.
	debug bool

	// printPrivate indicates that private keys should be printed.
	printPrivate bool

	// printFull indicates that full keys should be printed (not abbreviated).
	printFull bool

	// printBase58 indicates that keys should be printed in base58 format.
	printBase58 bool

	// printParent indicates that the parent key should be printed.
	printParent bool

	// useLedger indicates that the Ledger device should be used.
	useLedger bool

	// hrp is the human-readable network identifier used in Spacemesh network addresses.
	hrp string
)

func openWallet(walletFn string) (*wallet.Wallet, error) {
	// make sure the file exists
	f, err := os.Open(walletFn)
	cobra.CheckErr(err)
	defer f.Close()

	// get the password
	fmt.Print("Enter wallet password: ")
	password, err := password.Read(os.Stdin)
	fmt.Println()
	if err != nil {
		return nil, err
	}

	// attempt to read it
	wk := wallet.NewKey(wallet.WithPasswordOnly([]byte(password)))
	w, err := wk.Open(f, debug)
	if err != nil {
		return nil, err
	}

	return w, nil
}

// walletCmd represents the wallet command.
var walletCmd = &cobra.Command{
	Use:   "wallet",
	Short: "Wallet management",
}

// createCmd represents the create command.
var createCmd = &cobra.Command{
	Use:   "create [--ledger] [numaccounts]",
	Short: "Generate a new wallet file from a BIP-39-compatible mnemonic or Ledger device",
	Long: `Create a new wallet file containing one or more accounts using a BIP-39-compatible mnemonic
or a Ledger hardware wallet. If using a mnemonic you can choose to use an existing mnemonic or generate
a new, random mnemonic.

Add --ledger to instead read the public key from a Ledger device. If using a Ledger device please make
sure the device is connected, unlocked, and the Spacemesh app is open.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// get the number of accounts to create
		n := 1
		if len(args) > 0 {
			tmpN, err := strconv.ParseInt(args[0], 10, 16)
			cobra.CheckErr(err)
			n = int(tmpN)
		}

		var w *wallet.Wallet
		var err error

		// Short-circuit and check for a ledger device
		if useLedger {
			w, err = wallet.NewMultiWalletFromLedger(n)
			cobra.CheckErr(err)
			fmt.Println("Note that, when using a hardware wallet, the wallet file I'm about to produce won't " +
				"contain any private keys or mnemonics, but you may still choose to encrypt it to protect privacy.")
		} else {
			// get or generate the mnemonic
			fmt.Print("Enter a BIP-39-compatible mnemonic (or leave blank to generate a new one): ")
			text, err := password.Read(os.Stdin)
			fmt.Println()
			cobra.CheckErr(err)
			fmt.Print("Note: This application does not yet support BIP-39-compatible optional passwords. ")
			fmt.Println("Support will be added soon.")

			// It's critical that we trim whitespace, including CRLF. Otherwise it will get included in the mnemonic.
			text = strings.TrimSpace(text)

			if text == "" {
				w, err = wallet.NewMultiWalletRandomMnemonic(n)
				cobra.CheckErr(err)
				fmt.Print("\nThis is your mnemonic (seed phrase). Write it down and store it safely.")
				fmt.Print("It is the ONLY way to restore your wallet.\n")
				fmt.Print("Neither Spacemesh nor anyone else can help you restore your wallet without this mnemonic.\n")
				fmt.Print("\n***********************************\n")
				fmt.Print("SAVE THIS MNEMONIC IN A SAFE PLACE!")
				fmt.Print("\n***********************************\n")
				fmt.Println()
				fmt.Println(w.Mnemonic())
				fmt.Println("\nPress enter when you have securely saved your mnemonic.")
				_, _ = fmt.Scanln()
			} else {
				// try to use as a mnemonic
				w, err = wallet.NewMultiWalletFromMnemonic(text, n)
				cobra.CheckErr(err)
			}
		}

		fmt.Print("Enter a secure password used to encrypt the wallet file (optional but strongly recommended): ")
		password, err := password.Read(os.Stdin)
		fmt.Println()
		cobra.CheckErr(err)
		wk := wallet.NewKey(wallet.WithRandomSalt(), wallet.WithPbkdf2Password([]byte(password)))
		err = os.MkdirAll(common.DotDirectory(), 0o700)
		cobra.CheckErr(err)

		// Make sure we're not overwriting an existing wallet (this should not happen)
		walletFn := common.WalletFile()
		_, err = os.Stat(walletFn)
		switch {
		case errors.Is(err, os.ErrNotExist):
			// all fine
		case err == nil:
			log.Fatalln("Wallet file already exists")
		default:
			log.Fatalf("Error opening %s: %v\n", walletFn, err)
		}

		// Now open for writing
		f2, err := os.OpenFile(walletFn, os.O_WRONLY|os.O_CREATE, 0o600)
		cobra.CheckErr(err)
		defer f2.Close()
		cobra.CheckErr(wk.Export(f2, w))

		fmt.Printf("Wallet saved to %s. BACK UP THIS FILE NOW!\n", walletFn)
	},
}

// readCmd reads an existing wallet file.
var readCmd = &cobra.Command{
	Use:   "read [wallet file] [--full/-f] [--private/-p] [--base58]",
	Short: "Reads an existing wallet file",
	Long: `This command can be used to verify whether an existing wallet file can be
successfully read and decrypted, whether the password to open the file is correct, etc.
It prints the accounts from the wallet file. By default it does not print private keys.
Add --private to print private keys. Add --full to print full keys. Add --base58 to print
keys in base58 format rather than hexadecimal. Add --parent to print parent key (and not
only child keys).`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		walletFn := args[0]

		w, err := openWallet(walletFn)
		cobra.CheckErr(err)

		widthEnforcer := func(col string, maxLen int) string {
			if len(col) <= maxLen {
				return col
			}
			if maxLen <= 7 {
				return col[:maxLen]
			}
			return fmt.Sprintf("%s..%s", col[:maxLen-7], col[len(col)-5:])
		}

		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.SetTitle("Wallet Contents")
		caption := ""
		if printPrivate {
			caption = fmt.Sprintf("Mnemonic: %s", w.Mnemonic())
		}
		if !printFull {
			if printPrivate {
				caption += "\n"
			}
			caption += "To print full keys, use the --full flag."
		}
		t.SetCaption(caption)
		maxWidth := 20
		if printFull {
			// full key is 64 bytes which is 128 chars in hex, need to print at least this much
			maxWidth = 150
		}
		if printPrivate {
			t.AppendHeader(table.Row{
				"address",
				"pubkey",
				"privkey",
				"path",
				"name",
				"created",
			})
			t.SetColumnConfigs([]table.ColumnConfig{
				{Number: 2, WidthMax: maxWidth, WidthMaxEnforcer: widthEnforcer},
				{Number: 3, WidthMax: maxWidth, WidthMaxEnforcer: widthEnforcer},
			})
		} else {
			t.AppendHeader(table.Row{
				"address",
				"pubkey",
				"path",
				"name",
				"created",
			})
			t.SetColumnConfigs([]table.ColumnConfig{
				{Number: 2, WidthMax: maxWidth, WidthMaxEnforcer: widthEnforcer},
			})
		}

		// set the encoder
		encoder := hex.EncodeToString
		if printBase58 {
			encoder = base58.Encode
		}

		privKeyEncoder := func(privKey []byte) string {
			if len(privKey) == 0 {
				return "(none)"
			}
			return encoder(privKey)
		}

		// print the master account
		if printParent {
			master := w.Secrets.MasterKeypair
			if master != nil {
				if printPrivate {
					t.AppendRow(table.Row{
						"N/A",
						encoder(master.Public),
						privKeyEncoder(master.Private),
						master.Path.String(),
						master.DisplayName,
						master.Created,
					})
				} else {
					t.AppendRow(table.Row{
						"N/A",
						encoder(master.Public),
						master.Path.String(),
						master.DisplayName,
						master.Created,
					})
				}
			}
		}

		// print child accounts
		for _, a := range w.Secrets.Accounts {
			if printPrivate {
				t.AppendRow(table.Row{
					wallet.PubkeyToAddress(a.Public, hrp),
					encoder(a.Public),
					privKeyEncoder(a.Private),
					a.Path.String(),
					a.DisplayName,
					a.Created,
				})
			} else {
				t.AppendRow(table.Row{
					wallet.PubkeyToAddress(a.Public, hrp),
					encoder(a.Public),
					a.Path.String(),
					a.DisplayName,
					a.Created,
				})
			}
		}
		t.Render()
	},
}

var signCmd = &cobra.Command{
	Use:   "sign [wallet file] [message]",
	Short: "Signs a message using a wallet's first child key",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		walletFn := args[0]
		message := args[1]

		w, err := openWallet(walletFn)
		cobra.CheckErr(err)

		// Sign message using child account 0.
		child0 := w.Secrets.Accounts[0] // TODO: flag to select child
		sk0 := ed25519.PrivateKey(child0.Private)
		sig, err := sk0.Sign(nil, []byte(message), crypto.Hash(0))
		cobra.CheckErr(err)

		// Output signed message in a JSON format compatible with smapp's signing feature.
		type signedMessage struct {
			Text      string `json:"text"`
			Signature string `json:"signature"`
			PublicKey string `json:"publicKey"`
		}
		out := signedMessage{
			Text:      message,
			Signature: "0x" + hex.EncodeToString(sig),
			PublicKey: "0x" + hex.EncodeToString(child0.Public),
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(out)
	},
}

var balanceCmd = &cobra.Command{
	Use:   "balance [wallet file] [node uri]",
	Short: "Retrieve balance",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		walletFn := args[0]
		nodeURI := args[1]

		types.SetNetworkHRP(hrp)

		w, err := openWallet(walletFn)
		cobra.CheckErr(err)

		ctx := context.Background()

		nodeConn, err := grpc.NewClient(nodeURI, grpc.WithTransportCredentials(insecure.NewCredentials()))
		cobra.CheckErr(err)
		defer nodeConn.Close()

		globalStateClient := pb.NewGlobalStateServiceClient(nodeConn)

		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.SetTitle("Wallet Balances")
		t.AppendHeader(table.Row{
			"#",
			"address",
			"path",
			"name",
			"balance",
		})
		for idx, account := range w.Secrets.Accounts {
			address := wallet.PubkeyToAddress(account.Public, hrp)
			accountReq := pb.AccountRequest{AccountId: &pb.AccountId{Address: string(address)}}
			accountResp, err := globalStateClient.Account(ctx, &accountReq)
			cobra.CheckErr(err)
			t.AppendRow(table.Row{
				idx,
				wallet.PubkeyToAddress(account.Public, hrp),
				account.Path.String(),
				account.DisplayName,
				float64(accountResp.AccountWrapper.StateProjected.Balance.Value) / 1e9,
			})
		}
		t.Render()
	},
}

var spawnCmd = &cobra.Command{
	Use:   "spawn [wallet file] [node uri]",
	Short: "Spawn wallet",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		walletFn := args[0]
		nodeURI := args[1]

		types.SetNetworkHRP(hrp)

		w, err := openWallet(walletFn)
		cobra.CheckErr(err)

		senderAccount := w.Secrets.Accounts[0] // TODO: flag to select child

		ctx := context.Background()

		nodeConn, err := grpc.NewClient(nodeURI, grpc.WithTransportCredentials(insecure.NewCredentials()))
		cobra.CheckErr(err)
		defer nodeConn.Close()

		meshClient := pb.NewMeshServiceClient(nodeConn)
		meshResp, err := meshClient.GenesisID(ctx, &pb.GenesisIDRequest{})
		cobra.CheckErr(err)
		genesisID := types.Hash20(meshResp.GenesisId)

		nonce := uint64(0)

		tx := walletSdk.SelfSpawn(
			ed25519.PrivateKey(senderAccount.Private),
			nonce,
			sdk.WithGenesisID(genesisID),
		)

		txClient := pb.NewTransactionServiceClient(nodeConn)
		// txResp, _ := txClient.ParseTransaction(ctx, &api.ParseTransactionRequest{Transaction: tx})
		// cobra.CheckErr(err)

		sendResp, err := txClient.SubmitTransaction(ctx, &pb.SubmitTransactionRequest{Transaction: tx})
		cobra.CheckErr(err)

		fmt.Printf("Submitted spawn transaction! id=%s status=%d state=%s\n",
			hex.EncodeToString(sendResp.Txstate.Id.Id),
			sendResp.Status.Code,
			sendResp.Txstate.State.String(),
		)
	},
}

var sendCmd = &cobra.Command{
	Use:   "send [wallet file] [node uri] [recipient address] [smh amount]",
	Short: "Send transaction",
	Args:  cobra.ExactArgs(4),
	Run: func(cmd *cobra.Command, args []string) {
		walletFn := args[0]
		nodeURI := args[1]
		recipientAddressString := args[2]
		smhAmountString := args[3]

		types.SetNetworkHRP(hrp)

		w, err := openWallet(walletFn)
		cobra.CheckErr(err)

		nodeConn, err := grpc.NewClient(nodeURI, grpc.WithTransportCredentials(insecure.NewCredentials()))
		cobra.CheckErr(err)
		defer nodeConn.Close()

		recipientAddress, err := types.StringToAddress(recipientAddressString)
		cobra.CheckErr(err)

		smhAmount, err := strconv.ParseFloat(smhAmountString, 64) // TODO: use decimal
		cobra.CheckErr(err)

		senderAccount := w.Secrets.Accounts[0] // TODO: flag to select child
		senderAddress := wallet.PubkeyToAddress(senderAccount.Public, hrp)

		ctx := context.Background()

		meshClient := pb.NewMeshServiceClient(nodeConn)
		meshResp, err := meshClient.GenesisID(ctx, &pb.GenesisIDRequest{})
		cobra.CheckErr(err)
		genesisID := types.Hash20(meshResp.GenesisId)

		globalStateClient := pb.NewGlobalStateServiceClient(nodeConn)
		accountReq := pb.AccountRequest{AccountId: &pb.AccountId{Address: string(senderAddress)}}
		accountResp, err := globalStateClient.Account(ctx, &accountReq)
		cobra.CheckErr(err)
		nonce := accountResp.AccountWrapper.StateProjected.Counter

		tx := walletSdk.Spend(
			ed25519.PrivateKey(senderAccount.Private),
			recipientAddress,
			uint64(smhAmount*1e9), // TODO: use decimal
			nonce+1,
			sdk.WithGenesisID(genesisID),
		)

		txClient := pb.NewTransactionServiceClient(nodeConn)
		// txResp, _ := txClient.ParseTransaction(ctx, &api.ParseTransactionRequest{Transaction: tx})
		// cobra.CheckErr(err)

		sendResp, err := txClient.SubmitTransaction(ctx, &pb.SubmitTransactionRequest{Transaction: tx})
		cobra.CheckErr(err)

		fmt.Printf("Submitted spend transaction! id=%s status=%d state=%s\n",
			hex.EncodeToString(sendResp.Txstate.Id.Id),
			sendResp.Status.Code,
			sendResp.Txstate.State.String(),
		)
	},
}

func init() {
	rootCmd.AddCommand(walletCmd)
	walletCmd.AddCommand(createCmd)
	walletCmd.AddCommand(readCmd)
	walletCmd.AddCommand(signCmd)
	walletCmd.AddCommand(balanceCmd)
	walletCmd.AddCommand(spawnCmd)
	walletCmd.AddCommand(sendCmd)
	hrpFlags := pflag.NewFlagSet("", pflag.ContinueOnError)
	hrpFlags.StringVar(&hrp, "hrp", types.NetworkHRP(), "Set human-readable address prefix")
	readCmd.Flags().BoolVarP(&printPrivate, "private", "p", false, "Print private keys")
	readCmd.Flags().BoolVarP(&printFull, "full", "f", false, "Print full keys (no abbreviation)")
	readCmd.Flags().BoolVar(&printBase58, "base58", false, "Print keys in base58 (rather than hex)")
	readCmd.Flags().BoolVar(&printParent, "parent", false, "Print parent key (not only child keys)")
	readCmd.Flags().AddFlagSet(hrpFlags)
	readCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "enable debug mode")
	createCmd.Flags().BoolVarP(&useLedger, "ledger", "l", false, "Create a wallet using a Ledger device")
	balanceCmd.Flags().AddFlagSet(hrpFlags)
	spawnCmd.Flags().AddFlagSet(hrpFlags)
	sendCmd.Flags().AddFlagSet(hrpFlags)
}
