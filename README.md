# Nostdress

Federated Lightning Address Server (with NIP57 Zap support)

## How to run

1. Download the binary from the releases page (or compile with `go build` or if you want to compile for another system ,like a linux webserver:   `env GOOS=linux GOARCH=amd64 go build`   )
2. Set the following environment variables by placing a .env file in the folder or copy .env.example to .env and adjust it to your needs.

New parameters in this fork:

> The (`NOSTR_PRIVATE_KEY`) in the parameters should be in bench32 format (`nsec..`) and it should be a new one. It is needed to sign messages only, so does not need and should not be your main account key.

> (`NIP05`) will run a NIPO5 instance so that users that register with their NPUB can use their Lightning address for validation in Nostr as well. 

> (`GET_NOSTR_PROFILE`) is still experimental and should be left on false in case it causes errors. The idea is to provide the Nostr Profile picture to wallets that support it.

> (`FORWARD_URL`) can be left unset and satdress will work as before. If set, the main Page will be forwarded to the given url. The interface can then be accesed at domain.org/lnaddress

> (`ALLOW_REGISTRATION`) Activate/Deactivate Registration for users of the website (Default true)

> (`NOTIFY_NOSTR_USERS`) Allow setting to send DM notifications to users with npub (user can opt in/out if activated)

> (`LND_PRIVATE_ONLY`) should only be activated if satdress is used for a single user that relies on private LND channels only. 

> (`DB_DIR`) Specify directory to create or access database

> (`RELAYS`) Specify comma separate list of relays to push zap notes to, in addition to the zapped user relays.



```
HOST="0.0.0.0"
PORT=17423
DOMAIN="yourdomain.org"
SECRET="69420"
SITE_OWNER_NAME="yourname"
SITE_OWNER_URL="http://yourdomain.org"
SITE_NAME="@yourname"
NOSTR_PRIVATE_KEY="nsec123"
FORWARD_URL="/"
NIP05=true
GET_NOSTR_PROFILE=false
```

3. Start the app with `./nostdress` or `nohup ./nostdress &` for a background task
4. Serve the app to the world on your domain using whatever technique you're used to

## Multiple domains

Note that `DOMAIN` can be a single domain or a comma-separated list. When using multiple domains
you need to make sure "Host" HTTP header is forwarded to satdress process if you have some reverse-proxy).

If you come from an old installation everything should get migrated in a seamless way, but there is also a
`FORCE_MIGRATE` environment variable to force a migration (else this is done just the first time).

There is also a `GLOBAL_USERS` to make sure the user@ part is unique across all domains. But be warned that when enabling
this option, existing users won't work anymore (which is by design).

## Status of the Fork:
- NIP57 for Nostr ("Zaps") work when using an LNBits or LND backend, other backends (sparko, lnpay, eclair, commando) still need verification of payments in waitforinvoice.go by API calls in order to sign the zap on Nostr. (Help appreciated, because I can't test them)
- NIP05 support: If user added a npub, they can use lnaddress for Nostr NIP05 verificaton
- Acts as a Bot that sends Nostr messages to users when they receive a LN Payment (if set in options for Zaps with/without comments and non Zaps (lnaddress payments))
- Downloads Profile pictures when given npub key (for supported wallets, e.g. blue wallet) and GET_NOSTR_PROFILE=true
- Addded possibility to forward lightning addresses to existing ones (e.g. Wallet of Satoshi)
- Added possibility to add a forward main page, go to /lnaddress to add new users
- Added an alternative API '/api/easy' that deletes users and creates new name and pin for them
- Code needs some refactoring
- Needs proper testing (especially in multi-user environment)
