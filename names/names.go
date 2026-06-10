// Package names generates random usernames. Every player — human or AI — is
// assigned one of these at creation, so that display names can't be used to
// tell humans and AIs apart (e.g. in Turing test games).
package names

import (
	"fmt"
	"math/rand"
)

var adjectives = []string{
	"Amber", "Bold", "Brave", "Bright", "Calm", "Clever", "Cosmic", "Crimson",
	"Curious", "Daring", "Dusty", "Eager", "Electric", "Fuzzy", "Gentle",
	"Golden", "Happy", "Hidden", "Icy", "Jolly", "Keen", "Lucky", "Lunar",
	"Mellow", "Mighty", "Misty", "Nimble", "Noble", "Polar", "Quick", "Quiet",
	"Rapid", "Royal", "Rusty", "Sandy", "Sharp", "Silent", "Silver", "Sly",
	"Smooth", "Snowy", "Solar", "Stormy", "Sunny", "Swift", "Velvet", "Wild",
	"Witty",
}

var nouns = []string{
	"Badger", "Bear", "Beaver", "Bison", "Cobra", "Comet", "Coyote", "Crane",
	"Eagle", "Falcon", "Ferret", "Fox", "Gecko", "Hawk", "Heron", "Jaguar",
	"Koala", "Lemur", "Lynx", "Maple", "Marmot", "Meteor", "Moose", "Otter",
	"Owl", "Panda", "Panther", "Pebble", "Penguin", "Pine", "Puffin", "Rabbit",
	"Raccoon", "Raven", "River", "Robin", "Sparrow", "Squid", "Tiger", "Toucan",
	"Turtle", "Walrus", "Weasel", "Wolf", "Wombat", "Wren", "Yak", "Zebra",
}

// Random returns a username like "SilentOtter42". It uses the global rand
// source, which is safe for concurrent use.
func Random() string {
	return fmt.Sprintf("%s%s%d",
		adjectives[rand.Intn(len(adjectives))],
		nouns[rand.Intn(len(nouns))],
		rand.Intn(90)+10)
}
