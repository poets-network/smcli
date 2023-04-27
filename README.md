# smcli: Spacemesh Command-line Interface Tool

smcli is a simple command line tool that you can use to manage wallet files (in the future it may be expanded with additional functionality).

It currently supports the following features. Note that this documentation is not intended to be as complete as the built-in help documentation in the application itself, which fully documents all commands, flags, and features. Run `smcli -h` to see this documentation.

## Wallet

smcli allows you to read encrypted wallet files (including those created using Smapp and other compatible tools), and generate new wallet files.

### Reading

To read an encrypted wallet file, run `smcli wallet read <filename>`. You'll be prompted to enter the (optional) password used to encrypt the wallet file. If you enter the correct password, you'll see the contents of the wallet printed, including the accounts it contains. Include the flags `--full` to see full keys, and `--private` to see private keys and mnemonic in addition to public keys.

Note that you can read both wallet files created using `smcli` as well as those created using [Smapp](https://github.com/spacemeshos/smapp/) or any other tool that supports standard Spacemesh wallet format.

### Generation

To generate a new wallet, run `smcli wallet create`. The command will prompt you to enter a [BIP39-compatible mnemonic](https://github.com/bitcoin/bips/blob/master/bip-0039.mediawiki), or alternatively generate a new, random mnemonic for you. It will then prompt you to enter a password to encrypt the wallet file (optional but highly recommended) and will then generate an encrypted wallet file with one or more new keypairs.

Note that these keypairs (public and private key) are _not_ the same as Spacemesh wallet addresses. The public key can be converted directly and deterministically into your wallet address; in other words, there is a one-to-one mapping between public keys and wallet addresses. Conversion and outputting of public keys as wallet addresses [will be available shortly](https://github.com/spacemeshos/smcli/issues/38).

#### Hardware wallet support

Support for hardware wallets such as Ledger devices is not currently available in `smcli` but will be [added shortly](https://github.com/spacemeshos/smcli/issues/10).

**NOTE: We strongly recommend only creating a new wallet on a secure, airgapped computer. You are responsible for safely storing your mnemonic and wallet files. Your mnemonic is the ONLY way to restore access to your wallet and accounts if you misplace the wallet file, so it's essential that you back it up securely and reliably. There is absolutely nothing that we can do to help you recover your wallet if you misplace the file or mnemonic.**
