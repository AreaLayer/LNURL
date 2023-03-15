<a href="https://nbd.wtf"><img align="right" height="196" src="https://user-images.githubusercontent.com/1653275/194609043-0add674b-dd40-41ed-986c-ab4a2e053092.png" /></a>

# Satdress

Federated Lightning Address Server (with NIP57 Zap support)

## How to run

1. Download the binary from the releases page (or compile with `go build` or if you want to compile for another system ,like a linux webserver:   `env GOOS=linux GOARCH=amd64 go build`   )
2. Set the following environment variables somehow (using example values from bitmia.com):

> The Nostr private key in the parameters should be in bench32 format (`nsec..`) and it should be a new one. It is needed to sign messages only, so does not need to be your main account key.

(`GetNostrProfile`) is still experimental and should be left on false in case it causes errors. The idea is to provide the Nostr Profile picture to wallets that support it.

```
PORT=17422
DOMAIN=bitmia.com
SECRET=askdbasjdhvakjvsdjasd
SITE_OWNER_URL=https://t.me/qecez
SITE_OWNER_NAME=@qecez
SITE_NAME=Bitmia
NOSTR_PRIVATE_KEY=nsec213....
FORWARD_URL=https://thepageyouwanttoshowasmainpage.com
NIP05=true
GetNostrProfile=false
```

3. Start the app with `./satdress` or `nohup ./satdress &` for a background task
4. If you don't know how to set env you can put the above parameters in your commandline before `./satdress` 
5. Serve the app to the world on your domain using whatever technique you're used to

## Multiple domains

Note that `DOMAIN` can be a single domain or a comma-separated list. When using multiple domains
you need to make sure "Host" HTTP header is forwarded to satdress process if you have some reverse-proxy).

If you come from an old installation everything should get migrated in a seamless way, but there is also a
`FORCE_MIGRATE` environment variable to force a migration (else this is done just the first time).

There is also a `GLOBAL_USERS` to make sure the user@ part is unique across all domains. But be warned that when enabling
this option, existing users won't work anymore (which is by design).

## Get help

Maybe ask for help on https://t.me/lnurl if you're in trouble.


## Status of the Fork:
- NIP57 for Nostr ("Zaps") work when using an LNBits or LND backend, other backends (sparko, lnpay, eclair, commando) still need verification of payments in waitforinvoice.go by API calls in order to sign the zap on Nostr. (Help appreciated, because I can't test them)
- NIP05 support: If user added a npub, they can use lnaddress for Nostr NIP05 verificaton
- Downloads Profile pictures when given npub key (for supported wallets, e.g. blue wallet)
- Addded possibility to forward lightning addresses to existing ones (e.g. Wallet of Satoshi)
- Added possibility to add a forward main page, go to /lnaddress to add new users
- Added an alternative API '/api/easy' that deletes users and creates new name and pin for them
- Code needs some refactoring
- Needs proper testing (especially in multi-user environment)
- Credit for the inspiration by LightningTipBot code from @calle
https://github.com/LightningTipBot/LightningTipBot

Download latest Release https://github.com/believethehype/satdress/releases/latest
