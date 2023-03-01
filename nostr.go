package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
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

func Nip57DescriptionHash(zapEventSerialized string) string {
	hash := sha256.Sum256([]byte(zapEventSerialized))
	hashString := hex.EncodeToString(hash[:])
	return hashString
}

func DecodeBench32(key string) string {
	if _, v, err := nip19.Decode(s.NostrPrivateKey); err == nil {
		privatekeyhex := v.(string)
		nostrPrivkeyHex = privatekeyhex
		return nostrPrivkeyHex
	}
	return key

}

func publishNostrEvent(ev nostr.Event, relays []string) {
	pk := s.NostrPrivateKey
	ev.Sign(pk)
	log.Debug().Str("[NOSTR] 🟣 publishing nostr event %s", ev.ID)
	// more relays
	relays = append(relays, "wss://relay.nostr.ch", "wss://eden.nostr.land", "wss://nostr.btcmp.com", "wss://nostr.relayer.se", "wss://relay.current.fyi", "wss://nos.lol", "wss://nostr.mom", "wss://relay.nostr.info", "wss://nostr.zebedee.cloud", "wss://nostr-pub.wellorder.net", "wss://relay.snort.social/", "wss://relay.damus.io/", "wss://nostr.oxtr.dev/", "wss://nostr.fmt.wiz.biz/", "wss://brb.io")
	// remove trailing /
	relays = cleanUrls(relays)
	// unique relays
	relays = uniqueSlice(relays)

	// publish the event to relays
	for _, url := range relays {
		go func(url string) {
			// remove trailing /
			relay, e := nostr.RelayConnect(context.Background(), url)
			if e != nil {
				log.Error().Str(e.Error(), e.Error())
				return
			}
			time.Sleep(3 * time.Second)

			status := relay.Publish(context.Background(), ev)
			log.Info().Str("[NOSTR] published to %s:", status.String())

			time.Sleep(3 * time.Second)
			relay.Close()
		}(url)

	}
}
func uniqueSlice(slice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range slice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func cleanUrls(slice []string) []string {
	list := []string{}
	for _, entry := range slice {
		if strings.HasSuffix(entry, "/") {
			entry = entry[:len(entry)-1]
		}
		list = append(list, entry)
	}
	return list
}
