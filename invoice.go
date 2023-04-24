package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"strconv"
	"time"

	"github.com/nfnt/resize"

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

	/* if params.Npub != "" && s.GetNostrProfile {
		NostrProfile, err := GetNostrProfileMetaData(params.Npub)
		if err == nil {
			addImageToMetaData(&metadata, NostrProfile.Picture)
		}

	} */

	return metadata

}

// addImageToMetaData adds an image to the LNURL metadata
func addImageToMetaData(metadata *lnurl.Metadata, imageurl string) {
	// Download and resize profile picture
	picture, err := DownloadProfilePicture(imageurl)
	if err != nil {
		log.Debug().Str("Downloading profile picture", err.Error()).Msg("Error")
		return
	}

	// Determine image format
	contentType := http.DetectContentType(picture)
	var ext string
	if contentType == "image/jpeg" {
		ext = "jpeg"
	} else if contentType == "image/png" {
		ext = "png"
	} else {
		log.Debug().Str("Detecting image format", "unknown format").Msg("Error")
		return
	}

	// Set image metadata in LNURL metadata
	metadata.Image.Ext = ext
	metadata.Image.DataURI = "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(picture)
	metadata.Image.Bytes = picture
}

func DownloadProfilePicture(url string) ([]byte, error) {
	client := http.Client{
		Timeout: 5 * time.Second,
	}

	res, err := client.Get(url)
	if err != nil {
		return nil, errors.New("failed to download image: " + err.Error())
	}
	defer res.Body.Close()

	contentType := res.Header.Get("Content-Type")
	if contentType != "image/jpeg" && contentType != "image/png" {
		return nil, errors.New("unsupported image format")
	}

	var img image.Image
	switch contentType {
	case "image/jpeg":
		img, err = jpeg.Decode(res.Body)
	case "image/png":
		img, err = png.Decode(res.Body)
	}
	if err != nil {
		return nil, errors.New("failed to decode image: " + err.Error())
	}

	img = resize.Thumbnail(thumbnailWidth, thumbnailHeight, img, resize.Lanczos3)

	buf := new(bytes.Buffer)
	if contentType == "image/jpeg" {
		if err := jpeg.Encode(buf, img, nil); err != nil {
			return nil, errors.New("failed to encode image: " + err.Error())
		}
	} else if contentType == "image/png" {
		if err := png.Encode(buf, img); err != nil {
			return nil, errors.New("failed to encode image: " + err.Error())
		}
	}

	return buf.Bytes(), nil
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
