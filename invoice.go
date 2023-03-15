package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/nfnt/resize"

	"github.com/fiatjaf/go-lnurl"
	"github.com/fiatjaf/makeinvoice"
)

func metaData(params *Params) lnurl.Metadata {

	metadata := lnurl.Metadata{
		Description:      fmt.Sprintf("Pay to %s@%s", params.Name, params.Domain),
		LightningAddress: fmt.Sprintf("%s@%s", params.Name, params.Domain),
	}

	if params.Npub != "" && s.GetNostrProfile {
		NostrProfile, err := GetNostrProfileMetaData(params.Npub)
		if err == nil {
			addImageToMetaData(&metadata, NostrProfile.Picture)
		}

	}

	return metadata

}

// addImageMetaData add images an image to the LNURL metadata
func addImageToMetaData(metadata *lnurl.Metadata, imageurl string) {

	picture, err := DownloadProfilePicture(imageurl)
	if err != nil {
		log.Debug().Str("Downloading profile picture", err.Error()).Msg("Error")
		return
	}
	metadata.Image.Ext = "jpeg" //filepath.Ext(imageurl)
	metadata.Image.DataURI = imageurl
	metadata.Image.Bytes = picture
}

func DownloadProfilePicture(url string) ([]byte, error) {
	res, err := http.Get(url)

	if err != nil {
		log.Debug().Str("http.Get ->", err.Error()).Msg("Error")
		return nil, err
	}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Debug().Str("ioutil.ReadAll ->", err.Error()).Msg("Error")
		return nil, err
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
	//ioutil.WriteFile("test.jpg", buf.Bytes(), 0666)
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

	var backend makeinvoice.BackendParams
	switch params.Kind {
	case "sparko":
		backend = makeinvoice.SparkoParams{
			Host: params.Host,
			Key:  params.Key,
		}
	case "lnd":
		backend = makeinvoice.LNDParams{
			Host:     params.Host,
			Macaroon: params.Key,
		}
	case "lnbits":
		backend = makeinvoice.LNBitsParams{
			Host: params.Host,
			Key:  params.Key,
		}
	case "lnpay":
		backend = makeinvoice.LNPayParams{
			PublicAccessKey:  params.Pak,
			WalletInvoiceKey: params.Waki,
		}
	case "eclair":
		backend = makeinvoice.EclairParams{
			Host:     params.Host,
			Password: "",
		}
	case "commando":
		backend = makeinvoice.CommandoParams{
			Host:   params.Host,
			NodeId: params.NodeId,
			Rune:   params.Rune,
		}
	}

	mip := makeinvoice.Params{
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
	bolt11, err = makeinvoice.MakeInvoice(mip)

	log.Debug().Int("msatoshi", msat).
		Interface("backend", backend).
		Str("bolt11", bolt11).Err(err).Str("Description", mip.Description).
		Msg("invoice generation")

	return bolt11, err
}
