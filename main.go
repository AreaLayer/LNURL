package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/fiatjaf/makeinvoice"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	_ "github.com/lib/pq"
	"github.com/rs/cors"
	"github.com/rs/zerolog"
)

type Settings struct {
	Host        string `envconfig:"HOST" default:"0.0.0.0"`
	Port        string `envconfig:"PORT" required:"true"`
	Domain      string `envconfig:"DOMAIN" required:"true"`
	DBDirectory string `envconfig:"DB_DIR" required:"false" default:""`
	Relays      string `envconfig:"RELAYS" required:"false" default:""`
	// GlobalUsers means that user@ part is globally unique across all domains
	// WARNING: if you toggle this existing users won't work anymore for safety reasons!
	GlobalUsers        bool   `envconfig:"GLOBAL_USERS" default:"false"`
	Secret             string `envconfig:"SECRET" required:"true"`
	SiteOwnerName      string `envconfig:"SITE_OWNER_NAME" required:"true"`
	SiteOwnerURL       string `envconfig:"SITE_OWNER_URL" required:"true"`
	SiteName           string `envconfig:"SITE_NAME" required:"true"`
	NostrPrivateKey    string `envconfig:"NOSTR_PRIVATE_KEY" required:"false" default:""`
	ForwardMainPageUrl string `envconfig:"FORWARD_URL" required:"false"`
	Nip05              bool   `envconfig:"NIP05" default:"false" required:"false"`
	GetNostrProfile    bool   `envconfig:"GET_NOSTR_PROFILE" required:"false" default:"false"`
	ForceMigrate       bool   `envconfig:"FORCE_MIGRATE"  default:"false"`
	TorProxyURL        string `envconfig:"TOR_PROXY_URL"`
	NotifyNostrUsers   bool   `envconfig:"NOTIFY_NOSTR_USERS" required:"false" default:"true"`
	AllowRegistration  bool   `envconfig:"ALLOW_REGISTRATION" required:"false" default:"true"`
	LNDprivateOnly     bool   `envconfig:"LND_PRIVATE_ONLY" required:"false" default:"false"`
}

var (
	s      Settings
	db     *pebble.DB
	router = mux.NewRouter()
	log    = zerolog.New(os.Stderr).Output(zerolog.ConsoleWriter{Out: os.Stderr})
)

// array of additional relays
var Relays []string

//go:embed index.html
var indexHTML string

//go:embed grab.html
var grabHTML string

//go:embed static
var static embed.FS

func main() {

	godotenv.Load(".env")
	err := envconfig.Process("", &s)
	if err != nil {
		log.Fatal().Err(err).Msg("couldn't process envconfig.")
	}

	// parse our relays
	Relays = strings.Split(s.Relays, ",")
	// Check if relays are not specified and add our bootstrap relays
	if len(Relays) == 1 && Relays[0] == "" {
		Relays = []string{
			"wss://relay.damus.io",         // Main Damus Relay
			"wss://nostr.mutinywallet.com", // Special broadcast relay
			"wss://relay.nostrgraph.net",   // Special broadcast relay
			"wss://nos.lol",                // Large relay
			"wss://relay.snort.social",     // Large relay for snort Users
		}
	}

	// increase default makeinvoice client timeout because people are using tor
	makeinvoice.Client = &http.Client{Timeout: 25 * time.Second}

	s.Domain = strings.ToLower(s.Domain)

	if s.TorProxyURL != "" {
		makeinvoice.TorProxyURL = s.TorProxyURL
	}

	dbName := path.Join(s.DBDirectory, fmt.Sprintf("%v-multiple.db", s.SiteName))
	if _, err := os.Stat(dbName); os.IsNotExist(err) || s.ForceMigrate {
		for _, one := range getDomains(s.Domain) {
			tryMigrate(one, dbName)
		}
	}

	db, err = pebble.Open(dbName, nil)
	if err != nil {
		log.Fatal().Err(err).Str("path", dbName).Msg("failed to open db.")
	}

	router.Path("/.well-known/lnurlp/{user}").Methods("GET").
		HandlerFunc(handleLNURL)

	router.Path("/.well-known/nostr.json").Methods("GET").
		HandlerFunc(handleNip05)

	router.Path("/lnaddress").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			//renderHTML(w, indexHTML, map[string]interface{}{})
			renderHTML(w, indexHTML, struct {
				AllowRegistration string `json:"allowregistration"`
				NotifyNostrUsers  string `json:"notifynostr"`
			}{strconv.FormatBool(s.AllowRegistration), strconv.FormatBool(s.NotifyNostrUsers)})
		},
	)

	router.Path("/").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if s.ForwardMainPageUrl != "" {
				http.Redirect(w, r, s.ForwardMainPageUrl, http.StatusSeeOther)
			} else {
				http.Redirect(w, r, "/lnaddress", http.StatusSeeOther)
			}

		},
	)

	router.PathPrefix("/static/").Handler(http.FileServer(http.FS(static)))

	router.Path("/grab").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			name := r.FormValue("name")
			if name == "" || r.FormValue("kind") == "" {
				sendError(w, 500, "internal error")
				return
			}

			// might not get domain back
			domain := r.FormValue("domain")
			if domain == "" {
				if !strings.Contains(s.Domain, ",") {
					domain = s.Domain
				} else {
					sendError(w, 500, "internal error")
					return
				}
			}

			r.ParseForm()
			v1 := r.FormValue("notifyzaps")
			var notifyZaps = false
			if v1 == "on" {
				notifyZaps = true
			}
			v2 := r.FormValue("notifycomments")
			var notifyComments = false
			if v2 == "on" {
				notifyComments = true
			}
			v3 := r.FormValue("notifynonzaps")
			var notifyNonZaps = false
			if v3 == "on" {
				notifyNonZaps = true
			}
			pin, inv, err := SaveName(name, domain, &Params{
				Kind:             r.FormValue("kind"),
				Host:             r.FormValue("host"),
				Key:              r.FormValue("key"),
				Pak:              r.FormValue("pak"),
				Waki:             r.FormValue("waki"),
				NodeId:           r.FormValue("nodeid"),
				Rune:             r.FormValue("rune"),
				Npub:             r.FormValue("npub"),
				NotifyZaps:       notifyZaps,
				NotifyZapComment: notifyComments,
				NotifyNonZap:     notifyNonZaps,
			}, r.FormValue("pin"), false, "")
			if err != nil {
				w.WriteHeader(500)
				fmt.Fprint(w, err.Error())
				return
			}

			renderHTML(w, grabHTML, struct {
				PIN          string `json:"pin"`
				Invoice      string `json:"invoice"`
				Name         string `json:"name"`
				ActualDomain string `json:"actual_domain"`
			}{pin, inv, name, domain})
		},
	)

	//Alternative API function that brutally deletes previous user and gives them a new name and pin.
	//Also works when user didn't exist before.

	//Returns Status ok (bool) and pin needed to authorize next call. Save this and the name e.g. in a DB for the user.
	//http Post the following content to yourdomain/api/easy
	//Expected input with lnbits  example:
	//            { new StringContent(thecurrentnameyouwanttochange), "currentname" },
	//            { new StringContent(thenewname), "name" },
	//            { new StringContent(https://yoursatdressdomain.com), "domain" },
	//            { new StringContent("lnbits"), "kind" },
	//            { new StringContent("https://lnbits.yourdomain.com"), "host" },
	//            { new StringContent(lnbitsapikey), "key" },
	//            { new StringContent(pin), "pin" },

	router.Path("/api/easy/").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			newname := r.FormValue("name")
			currentName := r.FormValue("currentname")
			domain := r.FormValue("domain")
			currentPin := r.FormValue("pin")

			params := Params{
				Kind:   r.FormValue("kind"),
				Host:   r.FormValue("host"),
				Key:    r.FormValue("key"),
				Pak:    r.FormValue("pak"),
				Waki:   r.FormValue("waki"),
				NodeId: r.FormValue("nodeid"),
				Rune:   r.FormValue("rune"),
				Npub:   r.FormValue("npub"),
			}

			pin, _, err := SaveName(newname, domain, &params, currentPin, true, currentName)
			if err != nil {
				w.WriteHeader(500)
				fmt.Fprint(w, err.Error())
				return
			}
			params.Pin = pin

			response := ResponseEasy{
				Ok:  true,
				Pin: pin,
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(response)
		},
	)

	api := router.PathPrefix("/api/v1").Subrouter()
	api.Use(authenticate)

	// unauthenticated
	api.HandleFunc("/claim", ClaimAddress).Methods("POST")

	// authenticated routes; X-Pin in header or in json request body
	api.HandleFunc("/users/{name}@{domain}", GetUser).Methods("GET")
	api.HandleFunc("/users/{name}@{domain}", UpdateUser).Methods("PUT")
	api.HandleFunc("/users/{name}@{domain}", DeleteUser).Methods("DELETE")

	srv := &http.Server{
		Handler:      cors.Default().Handler(router),
		Addr:         s.Host + ":" + s.Port,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
	log.Debug().Str("addr", srv.Addr).Msg("listening")

	srv.ListenAndServe()
}

func getDomains(s string) []string {
	splitFn := func(c rune) bool {
		return c == ','
	}
	return strings.FieldsFunc(s, splitFn)
}
