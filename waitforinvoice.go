package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/fiatjaf/go-lnurl"
	decodepay "github.com/nbd-wtf/ln-decodepay"
)

func WaitForInvoicePaid(payvalues *lnurl.LNURLPayValues, params *Params) {
	//Check for a minute if invoice is paid
	//Do we have an easier way to do  this? How does it work for other backends than lnbits.

	var maxiterations = 60
	ticker := time.NewTicker(1 * time.Second)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:

				bolt11, _ := decodepay.Decodepay(payvalues.PR)

				switch params.Kind {
				case "sparko":
				case "lnd":
				case "lnbits":

					response, err := http.Get(params.Host + "/api/v1/payments/" + bolt11.PaymentHash)
					if err != nil {
						fmt.Print(err.Error())
					}
					responseData, err := ioutil.ReadAll(response.Body)
					if err != nil {
						fmt.Print(err.Error())
					}
					var jsonMap map[string]interface{}
					json.Unmarshal([]byte(string(responseData)), &jsonMap)
					//If invoice is paid and DescriptionHash matches Nip57 DescriptionHash, publish Zap Nostr Event
					if jsonMap["paid"].(bool) && bolt11.DescriptionHash == Nip57DescriptionHash(zapEventSerializedStr) {
						log.Debug().Str("ZAP", "Published on Nostr").Msg("zapped")
						publishNostrEvent(nip57Receipt, nip57ReceiptRelays)
						close(quit)
					}

				case "lnpay":
				case "eclair":
				case "commando":

				}

				if maxiterations == 0 {
					log.Debug().Str("ZAP", bolt11.PaymentHash).Msg("Timed out")
					close(quit)
				}
				maxiterations--
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
}
