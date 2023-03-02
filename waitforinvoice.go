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
	"strings"
	"time"

	"github.com/fiatjaf/go-lnurl"
	decodepay "github.com/nbd-wtf/ln-decodepay"
	"github.com/tidwall/gjson"
)

var (
	TorProxyURL = "socks5://127.0.0.1:9050"
	Client      = &http.Client{
		Timeout: 10 * time.Second,
	}
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

type CommandoParams struct {
	Rune   string
	Host   string
	NodeId string
}

func (l CommandoParams) getCert() string { return "" }
func (l CommandoParams) isTor() bool     { return strings.Index(l.Host, ".onion") != -1 }

type SparkoParams struct {
	Cert string
	Host string
	Key  string
}

func (l SparkoParams) getCert() string { return l.Cert }
func (l SparkoParams) isTor() bool     { return strings.Index(l.Host, ".onion") != -1 }

type LNDParams struct {
	Cert     string
	Host     string
	Macaroon string
}

func (l LNDParams) getCert() string { return l.Cert }
func (l LNDParams) isTor() bool     { return strings.Index(l.Host, ".onion") != -1 }

type LNBitsParams struct {
	Cert string
	Host string
	Key  string
}

func (l LNBitsParams) getCert() string { return l.Cert }
func (l LNBitsParams) isTor() bool     { return strings.Index(l.Host, ".onion") != -1 }

type LNPayParams struct {
	PublicAccessKey  string
	WalletInvoiceKey string
}

func (l LNPayParams) getCert() string { return "" }
func (l LNPayParams) isTor() bool     { return false }

type EclairParams struct {
	Host     string
	Password string
	Cert     string
}

func (l EclairParams) getCert() string { return l.Cert }
func (l EclairParams) isTor() bool     { return strings.Index(l.Host, ".onion") != -1 }

type StrikeParams struct {
	Key      string
	Username string
	Currency string
}

func (l StrikeParams) getCert() string { return "" }
func (l StrikeParams) isTor() bool     { return false }

type BackendParams interface {
	getCert() string
	isTor() bool
}

func WaitForInvoicePaid(payvalues *lnurl.LNURLPayValues, params *Params) {
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
		var maxiterations = 60
		ticker := time.NewTicker(1 * time.Second)
		quit := make(chan struct{})

		for {
			select {
			case <-ticker.C:

				bolt11, _ := decodepay.Decodepay(payvalues.PR)

				switch backend := mip.Backend.(type) {
				case SparkoParams:
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
					if resp.StatusCode >= 300 {
						body, _ := ioutil.ReadAll(resp.Body)
						text := string(body)
						if len(text) > 300 {
							text = text[:300]
						}
						return
					}

					b, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						fmt.Print(err.Error())
						return
					}

					if gjson.ParseBytes(b).Get("settled").String() == "true" && bolt11.DescriptionHash == Nip57DescriptionHash(zapEventSerializedStr) {
						log.Debug().Str("ZAP", "Published on Nostr").Msg("zapped")
						publishNostrEvent(nip57Receipt, nip57ReceiptRelays)
						close(quit)
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
					//If invoice is paid and DescriptionHash matches Nip57 DescriptionHash, publish Zap Nostr Event
					if jsonMap["paid"].(bool) && bolt11.DescriptionHash == Nip57DescriptionHash(zapEventSerializedStr) {
						log.Debug().Str("ZAP", "Published on Nostr").Msg("zapped")
						publishNostrEvent(nip57Receipt, nip57ReceiptRelays)
						close(quit)
					}

				case LNPayParams:
				case EclairParams:
				case CommandoParams:

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
