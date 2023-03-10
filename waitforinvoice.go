package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	decodepay "github.com/nbd-wtf/ln-decodepay"
	"github.com/tidwall/gjson"
)

type LNParams struct {
	Backend     BackendParams
	Msatoshi    int64
	Description string

	// setting this to true will cause .Description to be hashed and used as
	// the description_hash (h) field on the bolt11 invoice
	UseDescriptionHash bool

	Label string // only used for c-lightning
}

func WaitForInvoicePaid(payvalues LNURLPayValuesCustom, params *Params, comment string) {
	//Check for a minute if invoice is paid
	//Do we have an easier way to do  this? How does it work for other backends than lnbits.
	go func() {
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
			//Msatoshi: int64(msat),
			Backend: backend,

			Label: params.Domain + "/" + strconv.FormatInt(time.Now().Unix(), 16),
		}

		defer func(prevTransport http.RoundTripper) {
			Client.Transport = prevTransport
		}(Client.Transport)

		specialTransport := &http.Transport{}

		// use a cert or skip TLS verification?
		if mip.Backend.getCert() != "" {
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM([]byte(mip.Backend.getCert()))
			specialTransport.TLSClientConfig = &tls.Config{RootCAs: caCertPool}
		} else {
			specialTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}

		// use a tor proxy?
		if mip.Backend.isTor() {
			torURL, _ := url.Parse(TorProxyURL)
			specialTransport.Proxy = http.ProxyURL(torURL)
		}

		Client.Transport = specialTransport
		var maxiterations = 100
		ticker := time.NewTicker(1 * time.Second)
		quit := make(chan struct{})

		for {
			select {
			case <-ticker.C:

				bolt11, _ := decodepay.Decodepay(payvalues.PR)
				var isPaid bool = false

				switch backend := mip.Backend.(type) {
				case SparkoParams:
					//TODO
				case LNDParams:
					req, err := http.NewRequest("GET",
						backend.Host+"/v1/invoice/"+bolt11.PaymentHash,
						nil)
					if err != nil {
						fmt.Print(err.Error())
						return
					}
					if b, err := base64.StdEncoding.DecodeString(backend.Macaroon); err == nil {
						backend.Macaroon = hex.EncodeToString(b)
					}
					req.Header.Set("Grpc-Metadata-macaroon", backend.Macaroon)
					resp, err := Client.Do(req)
					if err != nil {
						fmt.Print(err.Error())
						return
					}
					defer resp.Body.Close()

					b, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						fmt.Print(err.Error())
						return
					}

					if gjson.ParseBytes(b).Get("settled").String() == "true" {
						isPaid = true
					}

				case LNBitsParams:

					response, err := http.Get(backend.Host + "/api/v1/payments/" + bolt11.PaymentHash)
					if err != nil {
						fmt.Print(err.Error())
						return
					}
					responseData, err := ioutil.ReadAll(response.Body)
					if err != nil {
						fmt.Print(err.Error())
						return
					}
					var jsonMap map[string]interface{}
					json.Unmarshal([]byte(string(responseData)), &jsonMap)

					if jsonMap["paid"].(bool) {
						isPaid = true
					}

				case LNPayParams:
					//TODO
				case EclairParams:
					//TODO
				case CommandoParams:
					//TODO

				}

				//If invoice is paid and DescriptionHash matches Nip57 DescriptionHash, publish Zap Nostr Event. This is rather a sanity check.
				if isPaid {

					log.Debug().Str("DescriptionHash", bolt11.DescriptionHash).Msg("zapped")
					log.Debug().Str("Description", bolt11.Description).Msg("zapped")
					var descriptionTag = *payvalues.nip57Receipt.Tags.GetFirst([]string{"description"})
					if bolt11.DescriptionHash == Nip57DescriptionHash(descriptionTag.Value()) {
						log.Debug().Str("ZAP", "Published on Nostr").Msg("zapped")
						publishNostrEvent(payvalues.nip57Receipt, payvalues.nip57ReceiptRelays)
						close(quit)
						return

					}
				}

				//Timeout waiting for payment after maxiterations
				if maxiterations == 0 {
					log.Debug().Str("NIP57 Bolt11", bolt11.PaymentHash).Msg("Timed out")
					close(quit)
					return
				}
				maxiterations--

			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
}
