package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/fiatjaf/go-lnurl"
	// "github.com/fiatjaf/makeinvoice"
)

func metaData(params *Params) lnurl.Metadata {

	//addImageToMetaData(w.telegram, &metadata, username, user.Telegram)
	return lnurl.Metadata{
		Description:      fmt.Sprintf("Pay to %s@%s", params.Name, params.Domain),
		LightningAddress: fmt.Sprintf("%s@%s", params.Name, params.Domain),
	}
}

func makeInvoice(
	params *Params,
	msat int,
	pin *string,
	zapEventSerializedStr string,
	comment string,
) (bolt11 string, err error) {
	// prepare params
	var backend BackendParams
	switch params.Kind {
	case "sparko":
		backend = SparkoParams{
			Host: params.Host,
			Key:  params.Key,
		}
	case "lnd":
		backend = LNDParams{
			Host:     params.Host,
			Macaroon: params.Key,
		}
	case "lnbits":
		backend = LNBitsParams{
			Host: params.Host,
			Key:  params.Key,
			Memo: comment,
		}
	case "lnpay":
		backend = LNPayParams{
			PublicAccessKey:  params.Pak,
			WalletInvoiceKey: params.Waki,
		}
	case "eclair":
		backend = EclairParams{
			Host:     params.Host,
			Password: "",
		}
	case "commando":
		backend = CommandoParams{
			Host:   params.Host,
			NodeId: params.NodeId,
			Rune:   params.Rune,
		}
	}

	mip := MIParams{
		Msatoshi: int64(msat),
		Backend:  backend,

		Label: params.Domain + "/" + strconv.FormatInt(time.Now().Unix(), 16),
	}

	if pin != nil {
		// use this as the description for new accounts
		mip.Description = fmt.Sprintf("%s's PIN for '%s@%s' lightning address: %s", params.Domain, params.Name, params.Domain, *pin)
	} else {
		//use zapEventSerializedStr if nip57, else build hash descriptionhash from params
		if zapEventSerializedStr != "" {
			//Ok here it gets a bit tricky (due to lack of standards, if we have a comment, we overwrite it)
			if comment != "" {
				mip.Memo = comment
				var zapEvent nostr.Event
				err = json.Unmarshal([]byte(zapEventSerializedStr), &zapEvent)
				if zapEvent.Content == "" {
					zapEvent.Content = comment
					zapEventSerialized, _ := json.Marshal(zapEvent)
					zapEventSerializedStr = fmt.Sprintf("%s", zapEventSerialized)
				}
			}

			mip.Description = zapEventSerializedStr

		} else {
			// make the lnurlpay description_hash
			mip.Description = metaData(params).Encode()
			mip.Memo = comment
		}

		mip.UseDescriptionHash = true

	}

	// actually generate the invoice
	bolt11, err = MakeInvoice(mip)

	log.Debug().Int("msatoshi", msat).
		Interface("backend", backend).
		Str("bolt11", bolt11).Err(err).Str("Description", mip.Description).
		Msg("invoice generation")

	return bolt11, err
}
