package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fiatjaf/go-lnurl"
	"github.com/gorilla/mux"
	"github.com/nbd-wtf/go-nostr"
	decodepay "github.com/nbd-wtf/ln-decodepay"
)

var allowNostr bool = false
var nostrPrivkeyHex string = ""
var nostrPubkey string = ""
var minSendable int = 1000
var maxSendable int = 1000000000
var CommentAllowed int = 500

type LNURLPayParamsCustom struct {
	lnurl.LNURLResponse
	Callback        string               `json:"callback"`
	Tag             string               `json:"tag"`
	MaxSendable     int64                `json:"maxSendable"`
	MinSendable     int64                `json:"minSendable"`
	EncodedMetadata string               `json:"metadata"`
	CommentAllowed  int64                `json:"commentAllowed"`
	PayerData       *lnurl.PayerDataSpec `json:"payerData,omitempty"`
	AllowsNostr     bool                 `json:"allowsNostr,omitempty"`
	NostrPubKey     string               `json:"nostrPubkey,omitempty"`
	Metadata        lnurl.Metadata       `json:"-"`
}

type LNURLPayValuesCustom struct {
	lnurl.LNURLResponse
	SuccessAction *lnurl.SuccessAction `json:"successAction"`
	Routes        interface{}          `json:"routes"` // ignored
	PR            string               `json:"pr"`
	Disposable    *bool                `json:"disposable,omitempty"`

	ParsedInvoice      decodepay.Bolt11 `json:"-"`
	PayerDataJSON      string           `json:"-"`
	nip57Receipt       nostr.Event      `json:"nip57Receipt"`
	nip57ReceiptRelays []string         `json:"nip57ReceiptRelays"`
}

func handleLNURL(w http.ResponseWriter, r *http.Request) {
	var err error
	var response interface{}

	username := mux.Vars(r)["user"]
	domains := getDomains(s.Domain)
	domain := ""

	if len(domains) == 1 {
		domain = domains[0]
	} else {
		hostname := r.URL.Host
		if hostname == "" {
			hostname = r.Host
		}

		for _, one := range getDomains(s.Domain) {
			if strings.Contains(hostname, one) {
				domain = one
				break
			}
		}
		if domain == "" {
			json.NewEncoder(w).Encode(lnurl.ErrorResponse("incorrect domain"))
			return
		}
	}

	params, err := GetName(username, domain)
	if err != nil {
		log.Error().Err(err).Str("name", username).Str("domain", domain).Msg("failed to get name")
		json.NewEncoder(w).Encode(lnurl.ErrorResponse(fmt.Sprintf(
			"failed to get name %s@%s", username, domain)))
		return
	}

	log.Info().Str("username", username).Str("domain", domain).Msg("got lnurl request")

	if amount := r.URL.Query().Get("amount"); amount == "" {
		// convert configured sendable amounts to integer
		minSendable, err := strconv.ParseInt(params.MinSendable, 10, 64)
		// set defaults
		if err != nil {
			minSendable = 1000
		}
		maxSendable, err := strconv.ParseInt(params.MaxSendable, 10, 64)
		if err != nil {
			maxSendable = 1000000000
		}

		// if a nostr private nsec key is set, set nostr nip57 flags
		if len(s.NostrPrivateKey) > 0 {
			//allows users to use nsec keys, work with hex internally.
			//This can be any private key, not necessarily from the user.
			nostrPrivkeyHex = DecodeBench32(s.NostrPrivateKey)
			allowNostr = true
			pk := nostrPrivkeyHex
			pub, _ := nostr.GetPublicKey(pk)
			nostrPubkey = pub
		}

		json.NewEncoder(w).Encode(LNURLPayParamsCustom{
			LNURLResponse:   lnurl.LNURLResponse{Status: "OK"},
			Callback:        fmt.Sprintf("https://%s/.well-known/lnurlp/%s", domain, username),
			MinSendable:     minSendable,
			MaxSendable:     maxSendable,
			EncodedMetadata: metaData(params).Encode(),
			CommentAllowed:  int64(CommentAllowed),
			Tag:             "payRequest",
			AllowsNostr:     allowNostr,
			NostrPubKey:     nostrPubkey,
		})

	} else {

		msat, err := strconv.Atoi(amount)
		if err != nil {
			json.NewEncoder(w).Encode(lnurl.ErrorResponse("amount is not integer"))
			return
		}

		// nostr NIP-57
		// the "nostr" query param has a zap request which is a nostr event
		// that specifies which nostr note has been zapped.
		// here we check wheter its present, the event signature is valid
		// and whether the event has the necessary tags that we need (p and relays are necessary, e is optional)

		zapEventQuery := r.FormValue("nostr")
		var zapEvent nostr.Event
		if len(zapEventQuery) > 0 {
			err = json.Unmarshal([]byte(zapEventQuery), &zapEvent)
			if err != nil {
				log.Error().Err(err).Str("[handleLnUrl] Couldn't parse nostr event: ", err.Error())

			} else {
				valid, err := zapEvent.CheckSignature()
				if !valid || err != nil {
					log.Error().Err(err).Str("[handleLnUrl] Nostr NIP-57 zap event signature invalid: ", err.Error())
					return
				}
				if len(zapEvent.Tags) == 0 || zapEvent.Tags.GetFirst([]string{"p"}) == nil {
					log.Error().Err(err).Str("[handleLnUrl] Nostr NIP-57 zap event validation error ", err.Error())
					return
				}

			}
		}

		comment := r.FormValue("comment")
		if len(comment) > CommentAllowed {
			log.Error().Err(err).Str("[handleLnUrl] Comment is too long", err.Error())
			return
		}

		// payer data
		payerdata := r.FormValue("payerdata")
		var payerData lnurl.PayerDataValues
		if len(payerdata) > 0 {
			err = json.Unmarshal([]byte(payerdata), &payerData)
			if err != nil {
				log.Error().Err(err).Str("[handleLnUrl] Couldn't parse payerdata", err.Error())
			}
		}

		//we outsource the second part in a function, we should do this for the first one too.
		response, err = serveLNURLpSecond(w, params, username, msat, comment, payerData, zapEvent)
		var payvaluescustom = response.(LNURLPayValuesCustom)
		//var payvalues = response.(*lnurl.LNURLPayValues)
		if err != nil {
			if response != nil {
				// there is a valid error response
				json.NewEncoder(w).Encode(response)
			}
			return
		}

		json.NewEncoder(w).Encode(lnurl.LNURLPayValues{
			LNURLResponse: payvaluescustom.LNURLResponse,
			PR:            payvaluescustom.PR,
			Routes:        payvaluescustom.Routes,
			SuccessAction: payvaluescustom.SuccessAction,
		})

		//err = json.NewEncoder(w).Encode(payvalues)
		if err != nil {
			json.NewEncoder(w).Encode(response)
			return
		}

		if allowNostr {
			go WaitForInvoicePaid(payvaluescustom, params)
		}
	}
}

func serveLNURLpSecond(w http.ResponseWriter, params *Params, username string, amount_msat int, comment string, payerData lnurl.PayerDataValues, zapEvent nostr.Event) (LNURLPayValuesCustom, error) {
	log.Debug().Any("Serving invoice for user %s", username)
	if amount_msat < minSendable || amount_msat > maxSendable {
		// amount is not ok
		return LNURLPayValuesCustom{
			LNURLResponse: lnurl.LNURLResponse{
				Status: "Error",
				Reason: fmt.Sprintf("Amount out of bounds (min: %d sat, max: %d sat).", minSendable/1000, maxSendable/1000)},
		}, fmt.Errorf("amount out of bounds")
	}

	// NIP57 ZAPs
	// for nip57 use the nostr event as the descriptionHash

	if zapEvent.Sig != "" {
		// we calculate the descriptionHash here, create an invoice with it
		// and store the invoice in the zap receipt later down the line
		zapEventSerialized, err := json.Marshal(zapEvent)
		zapEventSerializedStr = fmt.Sprintf("%s", zapEventSerialized)
		if err != nil {
			//log.Println(err)
			return LNURLPayValuesCustom{
				LNURLResponse: lnurl.LNURLResponse{
					Status: "Error",
					Reason: "Couldn't serialize zap event."},
			}, err
		}
		// we extract the relays from the zap request
		nip57ReceiptRelaysTags := zapEvent.Tags.GetFirst([]string{"relays"})
		if len(fmt.Sprintf("%s", nip57ReceiptRelaysTags)) > 0 {
			nip57ReceiptRelays = strings.Split(fmt.Sprintf("%s", nip57ReceiptRelaysTags), " ")
			// this tirty method returns slice [ "[relays", "wss...", "wss...", "wss...]" ] – we need to clean it up
			if len(nip57ReceiptRelays) > 1 {
				// remove the first entry
				nip57ReceiptRelays = nip57ReceiptRelays[1:]
				// clean up the last entry
				len_last_entry := len(nip57ReceiptRelays[len(nip57ReceiptRelays)-1])
				nip57ReceiptRelays[len(nip57ReceiptRelays)-1] = nip57ReceiptRelays[len(nip57ReceiptRelays)-1][:len_last_entry-1]
			}
		}
		// calculate description hash from the serialized nostr event in makeinvoice.go
		params.zapEventSerializedStr = zapEventSerializedStr
	} else {

		params.zapEventSerializedStr = ""
	}

	var response LNURLPayValuesCustom
	invoice, err := makeInvoice(params, amount_msat, nil)
	if err != nil {
		err = fmt.Errorf("Couldn't create invoice: %v", err.Error())
		response = LNURLPayValuesCustom{
			LNURLResponse: lnurl.LNURLResponse{
				Status: "Error",
				Reason: "Couldn't create invoice."},
		}
		return response, err
	}
	// nip57 - we need to store the newly created invoice in the zap receipt
	if zapEvent.Sig != "" {
		pk := nostrPrivkeyHex
		pub, _ := nostr.GetPublicKey(pk)
		nip57Receipt = nostr.Event{
			PubKey:    pub,
			CreatedAt: time.Now(),
			Kind:      9735,
			Tags: nostr.Tags{
				*zapEvent.Tags.GetFirst([]string{"p"}),
				[]string{"bolt11", invoice},
				[]string{"description", zapEventSerializedStr},
			},
		}
		if zapEvent.Tags.GetFirst([]string{"e"}) != nil {
			nip57Receipt.Tags = nip57Receipt.Tags.AppendUnique(*zapEvent.Tags.GetFirst([]string{"e"}))
		}
		nip57Receipt.Sign(pk)
	}

	return LNURLPayValuesCustom{
		LNURLResponse:      lnurl.LNURLResponse{Status: "OK"},
		PR:                 invoice,
		Routes:             make([]struct{}, 0),
		SuccessAction:      &lnurl.SuccessAction{Message: "Payment Received!", Tag: "message"},
		nip57Receipt:       nip57Receipt,
		nip57ReceiptRelays: nip57ReceiptRelays,
	}, nil

}
