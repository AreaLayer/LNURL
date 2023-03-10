package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nfnt/resize"

	"github.com/fiatjaf/go-lnurl"
	// "github.com/fiatjaf/makeinvoice"
)

func metaData(params *Params) lnurl.Metadata {

	metadata := lnurl.Metadata{
		Description:      fmt.Sprintf("Pay to %s@%s", params.Name, params.Domain),
		LightningAddress: fmt.Sprintf("%s@%s", params.Name, params.Domain),
	}

	NostrProfile, _ := GetNostrProfileMetaData(params.Npub)

	addImageToMetaData(&metadata, NostrProfile.Picture)
	return metadata

}

// addImageMetaData add images an image to the LNURL metadata
func addImageToMetaData(metadata *lnurl.Metadata, imageurl string) {

	picture, _ := DownloadProfilePicture(imageurl)
	// if err != nil {
	// 	log.Debug().Str("Downloading profile picture", err.Error()).Msg("Error")
	// 	return
	// }
	metadata.Image.Ext = "jpeg"
	metadata.Image.DataURI = imageurl
	metadata.Image.Bytes = picture
}

func DownloadProfilePicture(url string) ([]byte, error) {
	res, err := http.Get(url)

	if err != nil {
		//log.Fatalf("http.Get -> %v", err)
	}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		// log.Fatalf("ioutil.ReadAll -> %v", err)
	}
	res.Body.Close()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	// resize image
	img = resize.Thumbnail(160, 160, img, resize.Lanczos3)
	buf := new(bytes.Buffer)
	_ = jpeg.Encode(buf, img, nil)
	ioutil.WriteFile("test.jpg", buf.Bytes(), 0666)
	return buf.Bytes(), nil
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
