package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip04"
	"github.com/nbd-wtf/go-nostr/nip19"
)

type Tag []string
type Tags []Tag
type NostrEvent struct {
	ID        string    `json:"id"`
	PubKey    string    `json:"pubkey"`
	CreatedAt time.Time `json:"created_at"`
	Kind      int       `json:"kind"`
	Tags      Tags      `json:"tags"`
	Content   string    `json:"content"`
	Sig       string    `json:"sig"`
}

var nip57Receipt nostr.Event
var zapEventSerializedStr string
var nip57ReceiptRelays []string

// Relay connections
type RelayConnection struct {
	URL       string
	relay     *nostr.Relay
	lastUsed  time.Time
	closeChan chan bool
}

var relayConnections = make(map[string]*RelayConnection)
var relayConnectionsMutex sync.Mutex
var connectionTimeout = 30 * time.Minute
var ignoreRelayDuration = 5 * time.Minute
var ignoredRelays = make(map[string]time.Time)
var ignoredRelaysMutex sync.Mutex

func Nip57DescriptionHash(zapEventSerialized string) string {
	hash := sha256.Sum256([]byte(zapEventSerialized))
	hashString := hex.EncodeToString(hash[:])
	return hashString
}

func DecodeBench32(key string) string {
	if _, v, err := nip19.Decode(key); err == nil {
		return v.(string)
	}
	return key

}

func EncodeBench32Public(key string) string {
	if v, err := nip19.EncodePublicKey(key); err == nil {
		return v
	}
	return key
}

func EncodeBench32Private(key string) string {
	if v, err := nip19.EncodePrivateKey(key); err == nil {
		return v
	}
	return key
}

func EncodeBench32Note(key string) string {
	if v, err := nip19.EncodeNote(key); err == nil {
		return v
	}
	return key
}

func sendMessage(receiverKey string, message string) {

	var relays []string
	var tags nostr.Tags
	reckey := DecodeBench32(receiverKey)
	tags = append(tags, nostr.Tag{"p", reckey})

	//references, err := optSlice(opts, "--reference")
	//if err != nil {
	//	return
	//}
	//for _, ref := range references {
	//tags = append(tags, nostr.Tag{"e", reckey})
	//}

	// parse and encrypt content
	privkeyhex := DecodeBench32(s.NostrPrivateKey)
	pubkey, _ := nostr.GetPublicKey(privkeyhex)

	sharedSecret, err := nip04.ComputeSharedSecret(reckey, privkeyhex)
	if err != nil {
		log.Printf("Error computing shared key: %s. x\n", err.Error())
		return
	}

	encryptedMessage, err := nip04.Encrypt(message, sharedSecret)
	if err != nil {
		log.Printf("Error encrypting message: %s. \n", err.Error())
		return
	}

	event := nostr.Event{
		PubKey:    pubkey,
		CreatedAt: time.Now(),
		Kind:      nostr.KindEncryptedDirectMessage,
		Tags:      tags,
		Content:   encryptedMessage,
	}
	event.Sign(privkeyhex)
	publishNostrEvent(event, relays)
	log.Printf("%+v\n", event)
}

func handleNip05(w http.ResponseWriter, r *http.Request) {
	var err error
	var response string

	var allusers []Params
	allusers, err = GetAllUsers(s.Domain)
	firstpartstring := "{\n  \"names\": {\n"
	finalpartstring := " \t}\n}"
	var middlestring = ""

	for _, user := range allusers {
		nostrnpubHex := DecodeBench32(user.Npub)
		if user.Npub != "" { //do some more validation checks
			middlestring = middlestring + "\t\"" + user.Name + "\"" + ": " + "\"" + nostrnpubHex + "\"" + ",\n"
		}
	}

	if s.Nip05 {
		//Remove ',' from last entry
		if len(middlestring) > 2 {
			middlestringtrim := middlestring[:len(middlestring)-2]
			middlestringtrim += "\n"

			response = firstpartstring + middlestringtrim + finalpartstring
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintf(w, response)
	} else {
		return
	}

	if err != nil {
		return
	}
}

func GetNostrProfileMetaData(npub string) (nostr.ProfileMetadata, error) {
	ctx, _ := context.WithTimeout(context.Background(), 3*time.Second)

	var metadata *nostr.ProfileMetadata
	// connect to any relay
	url := "wss://relay.damus.io"
	relay, err := nostr.RelayConnect(ctx, url)
	if err != nil {
		return *metadata, err
	}

	// create filters
	var filters nostr.Filters
	if _, v, err := nip19.Decode(npub); err == nil {
		t := make(map[string][]string)
		t["p"] = []string{v.(string)}
		filters = []nostr.Filter{{
			Authors: []string{v.(string)},
			Kinds:   []int{0},
			// limit = 3, get the three most recent notes
			Limit: 1,
		}}
	} else {
		return *metadata, err

	}
	sub, err := relay.Subscribe(ctx, filters)
	evs := make([]nostr.Event, 0)

	go func() {
		<-sub.EndOfStoredEvents

	}()

	for ev := range sub.Events {

		evs = append(evs, *ev)
	}
	relay.Close()

	if len(evs) > 0 {
		metadata, err = nostr.ParseMetadata(evs[0])
	} else {
		err = fmt.Errorf("no profile found for npub %s on relay %s", npub, url)
	}
	return *metadata, err

}

func ignoreRelay(url string) {
	ignoredRelaysMutex.Lock()
	defer ignoredRelaysMutex.Unlock()
	ignoredRelays[url] = time.Now()
}

func isRelayIgnored(url string) bool {
	ignoredRelaysMutex.Lock()
	defer ignoredRelaysMutex.Unlock()

	if t, ok := ignoredRelays[url]; ok {
		if time.Since(t) < ignoreRelayDuration {
			return true
		}
		delete(ignoredRelays, url)
	}
	return false
}

func isBrokenPipeError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		if strings.Contains(netErr.Error(), "write: broken pipe") {
			return true
		}
	}
	return false
}

func getRelayConnection(url string) (*nostr.Relay, error) {
	if isRelayIgnored(url) {
		return nil, fmt.Errorf("relay %s is being ignored", url)
	}

	relayConnectionsMutex.Lock()
	defer relayConnectionsMutex.Unlock()

	if relayConn, ok := relayConnections[url]; ok {
		relayConn.lastUsed = time.Now()
		return relayConn.relay, nil
	}

	ctx := context.WithValue(context.Background(), "url", url)
	relay, err := nostr.RelayConnect(ctx, url)
	if err != nil {
		ignoreRelay(url)
		return nil, err
	}

	relayConn := &RelayConnection{
		URL:       url,
		relay:     relay,
		lastUsed:  time.Now(),
		closeChan: make(chan bool),
	}
	relayConnections[url] = relayConn

	go func() {
		select {
		case <-time.After(connectionTimeout):
			relayConnectionsMutex.Lock()
			if time.Since(relayConn.lastUsed) >= connectionTimeout {
				relay.Close()
				delete(relayConnections, url)
			}
			relayConnectionsMutex.Unlock()
		case <-relayConn.closeChan:
		}
	}()

	return relay, nil
}

func closeRelayConnection(url string) {
	relayConnectionsMutex.Lock()
	defer relayConnectionsMutex.Unlock()

	if relayConn, ok := relayConnections[url]; ok {
		relayConn.closeChan <- true
		relayConn.relay.Close()
		delete(relayConnections, url)
	}
}

func publishNostrEvent(ev nostr.Event, relays []string) {
	// Add more relays, remove trailing slashes, and ensure unique relays
	relays = uniqueSlice(cleanUrls(append(relays, Relays...)))

	ev.Sign(s.NostrPrivateKey)

	var wg sync.WaitGroup
	wg.Add(len(relays))

	// Publish the event to relays
	for _, url := range relays {
		go func(url string) {
			defer wg.Done()

			var err error
			var relay *nostr.Relay
			var status nostr.Status
			maxRetries := 3

			for i := 0; i < maxRetries; i++ {
				relay, err = getRelayConnection(url)
				if err != nil {
					log.Printf("Error connecting to relay %s: %v", url, err)
					return
				}

				time.Sleep(3 * time.Second)

				ctx := context.WithValue(context.Background(), "url", url)
				status, err = relay.Publish(ctx, ev)
				if err != nil {
					log.Printf("Error publishing to relay %s: %v", url, err)

					if isBrokenPipeError(err) {
						closeRelayConnection(url) // Close the broken connection
						continue                  // Retry connection and publish
					}
				} else {
					log.Printf("[NOSTR] published to %s: %s", url, status.String()) // Convert the nostr.Status value to a string
					break
				}

				time.Sleep(3 * time.Second)
			}
		}(url)
	}

	wg.Wait()
}

func ExtractNostrRelays(zapEvent nostr.Event) []string {
	relaysTag := zapEvent.Tags.GetFirst([]string{"relays"})
	log.Printf("Zap relaysTag: %s", relaysTag)

	if relaysTag == nil || len(*relaysTag) == 0 {
		return []string{}
	}

	// Skip the first element, which is the tag name
	relays := (*relaysTag)[1:]
	log.Printf("Zap relays: %v", relays)

	return relays
}

func CreateNostrReceipt(zapEvent nostr.Event, invoice string) (nostr.Event, error) {
	pub, err := nostr.GetPublicKey(nostrPrivkeyHex)
	if err != nil {
		return nostr.Event{}, err
	}

	zapEventSerialized, err := json.Marshal(zapEvent)
	if err != nil {
		return nostr.Event{}, err
	}

	nip57Receipt := nostr.Event{
		PubKey:    pub,
		CreatedAt: time.Now(),
		Kind:      9735,
		Tags: nostr.Tags{
			*zapEvent.Tags.GetFirst([]string{"p"}),
			[]string{"bolt11", invoice},
			[]string{"description", string(zapEventSerialized)},
		},
	}

	if eTag := zapEvent.Tags.GetFirst([]string{"e"}); eTag != nil {
		nip57Receipt.Tags = nip57Receipt.Tags.AppendUnique(*eTag)
	}

	err = nip57Receipt.Sign(nostrPrivkeyHex)
	if err != nil {
		return nostr.Event{}, err
	}

	return nip57Receipt, nil
}

func uniqueSlice(slice []string) []string {
	keys := make(map[string]bool)
	list := make([]string, 0, len(slice))
	for _, entry := range slice {
		if _, exists := keys[entry]; !exists && entry != "" {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func cleanUrls(slice []string) []string {
	list := make([]string, 0, len(slice))
	for _, entry := range slice {
		if strings.HasSuffix(entry, "/") {
			entry = entry[:len(entry)-1]
		}
		list = append(list, entry)
	}
	return list
}
