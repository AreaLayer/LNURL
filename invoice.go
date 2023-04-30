package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/fiatjaf/go-lnurl"
)

const (
	thumbnailWidth  = 160
	thumbnailHeight = 160
)

func metaData(params *Params) lnurl.Metadata {

	metadata := lnurl.Metadata{
		Description:      fmt.Sprintf("Pay to %s@%s", params.Name, params.Domain),
		LightningAddress: fmt.Sprintf("%s@%s", params.Name, params.Domain),
	}

	if params.Npub != "" && s.GetNostrProfile {
		if params.Image.DataURI != "" {
			metadata.Image.Bytes = params.Image.Bytes
			metadata.Image.Ext = params.Image.Ext
			metadata.Image.DataURI = params.Image.DataURI
		}

	}

	return metadata

}

func makeInvoice(params *Params, msat int, pin *string, zapEventSerializedStr string, comment string) (bolt11 string, err error) {
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

	mip := LNParams{
		Msatoshi: int64(msat),
		Backend:  backend,

		Label: params.Domain + "/" + strconv.FormatInt(time.Now().Unix(), 16),
	}

	if pin != nil {
		// use this as the description for new accounts
		mip.UseDescriptionHash = false
		mip.Description = fmt.Sprintf("%s's PIN for '%s@%s' lightning address: %s", params.Domain, params.Name, params.Domain, *pin)
	} else {
		//use zapEventSerializedStr if nip57,
		mip.UseDescriptionHash = true
		if zapEventSerializedStr != "" {
			mip.Description = zapEventSerializedStr

		} else {
			//else build hash descriptionhash from params
			mip.Description = metaData(params).Encode()
		}

	}

	// actually generate the invoice
	bolt11, err = MakeInvoice(mip)

	log.Debug().Int("msatoshi", msat).
		Interface("backend", backend).
		Str("bolt11", bolt11).Err(err).Str("Description", mip.Description).
		Msg("invoice generation")

	return bolt11, err
}
